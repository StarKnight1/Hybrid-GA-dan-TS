# SMP Mater Dei — Scheduling Algorithm Handoff

This document is for the next AI agent working on this codebase.
Read it fully before touching anything in `internal/algorithm/`.

---

## What This Project Does

This is a school timetable scheduling backend for SMP Mater Dei.
The core problem: assign every teaching assignment (class × subject × teacher × JP total)
to a (Day, StartSlot) in a weekly grid, with no hard conflicts and minimal soft violations.

**JP** = Jam Pelajaran (teaching period), 40 minutes each.
**Hard constraints:** no class in two places at once, no teacher in two places at once.
**Soft constraints:** don't split the same subject across two days in one week; prefer PJOK in the morning.

---

## Algorithm Architecture

The scheduler uses a **GA + SA hybrid**, implemented from scratch in `internal/algorithm/`.

```
Teaching Assignments
        ↓
GenerateMatrixBlocks()       — splits JP totals into MatrixBlocks (duration 1/2/3 JP)
        ↓
BuildCandidateIndex()        — pre-computes valid (Day, StartSlot) positions per block
        ↓
RunGA()                      — Genetic Algorithm: global search, population of chromosomes
        ↓
RunSA()                      — Simulated Annealing: local search on the best GA result
        ↓
ScheduleMatrix               — final placement, queried to build API response
```

`RunHybrid()` in `hybrid.go` orchestrates the two phases and supports multiple restarts.

---

## File-by-File Guide

### SAFE TO TOUCH — Low risk, well-isolated

#### `internal/algorithm/genetic_algorithm.go`
Default GA parameters live here in `DefaultGAConfig()`.
Safe to tune: `PopulationSize`, `Generations`, `MutationRate`, `EliteCount`, `TournamentSize`, `StagnationLimit`.
These are starting points chosen by convention, not from a domain-specific study.
No invariants at risk from changing numeric values.

#### `internal/algorithm/simulated_annealing.go` — PARTIAL
Default SA parameters live here in `DefaultSAConfig()`.
Safe to tune: `InitialTemperature`, `CoolingRate`, `Iterations`, `PerturbCount`, `PerturbAfter`.

**NOT safe to touch inside `RunSA()`:**
The group invariant logic (see Critical Invariants below). Specifically:
- The `if _, ok := groupByID[idA]; ok { continue }` swap-skip guard (around line 441)
- The `if _, isGroupMember := groupByID[displacedID]; isGroupMember { break }` in case 1
- The `if _, ok := groupByID[d1]; ok { break }` guards in case 2
- The `findFreeCandidateForGroup` call and its surrounding logic
- The perturbation group eviction logic (`toEvict = groupIDs`)

Removing any of these breaks the SBP group invariant (see below) and causes permanent unplaced blocks.

#### `internal/schedule/model.go`
API response structs. Safe to add new fields.
`SoftBreakdown` and `GABreakdown` are the soft/hard violation breakdown types.
Do not rename existing JSON keys — frontend depends on them.

#### `internal/schedule/service.go`
Wires algorithm results to API responses. Safe to change parameter defaults surfaced to the API.
`DefaultGAParams()` and `DefaultSAParams()` mirror the algorithm defaults for the API response.
If you change algorithm defaults, update these too.

#### `internal/algorithm/legacy/`
The old algorithm (pre-matrix). Not used by any active endpoint.
Safe to ignore entirely. Do not delete — it is referenced in the thesis.

---

### UNDERSTAND BEFORE TOUCHING — Core algorithm, high risk

#### `internal/algorithm/chromosome.go`
The most important file. Contains:

- **`Gene`** — `(Day string, StartSlot int)`. Zero value = unplaced.
- **`Chromosome`** — fixed-length slice of genes, index i = gene for blocks[i].
- **`BuildCandidateIndex`** — pre-computes valid positions per block. Called once. Uses `DaySlots` grid to check that every slot in the block's duration window is non-blocked.
- **`RandomChromosome`** — group-aware random init. When a block has a `GroupKey`, ALL members of the group are assigned the same gene in one shot. The `assigned` map prevents double-processing.
- **`UniformCrossover`** — same group-awareness: all members of a group inherit from the same parent.
- **`MutateGene`** — mutates a single gene. Callers (not this function) are responsible for propagating to group members.
- **`DecodeChromosome`** — translates a chromosome into a `ScheduleMatrix` by placing blocks in array order. **Array order = placement priority.** Grouped blocks are placed first in the array (see `GenerateMatrixBlocks`) so they claim slots before ungrouped blocks.
- **`CountSoftViolations`** — hot path. Called ~100k+ times per run. Uses only int counters and map[string]int. Do NOT add slice allocations or struct allocations here.
- **`BreakdownSoftViolations`** — reporting only, called once at the end. Returns per-category breakdown including `SameDaySplitGrouped` to identify SBP-related violations.
- **`SoftViolationBreakdown.Total()`** — weighted total: SameDaySplit×1 + PJOKAfterDeadline×3.

**PJOK soft constraint:** the 10:50 morning deadline is a soft constraint (weight 3), NOT a hard filter. Do not add any PJOK filtering back into `BuildCandidateIndex`. The hard filter was removed because it made the problem mathematically infeasible (24 JP needed in morning window, only 23 slots available).

#### `internal/algorithm/matrix_block_builder.go`
- **`GenerateMatrixBlocks`** — converts assignments into `MatrixBlock` slices. Grouped blocks (SBP) go first in the returned slice. This ordering is load-bearing for decode priority. Do not change the `append(grouped, ungrouped...)` ordering.
- **`SplitAssignmentJP`** — maps JP totals to duration slices. e.g. 4JP → [2,2], 6JP → [3,3]. PJOK 3JP → [2,1] (practice + theory split).
- **`BuildGroupIndex`** — maps GroupKey → slice of block indices. Used by GA and SA for group-aware operations.

#### `internal/algorithm/matrix_block.go`
`MatrixBlock` struct. Fields:
- `ID uuid.UUID` — deterministic (SHA1 of assignment+part+duration). Same inputs always produce same ID.
- `TeacherID *uuid.UUID` — nil for SBP blocks. Nil means no teacher conflict checking.
- `GroupKey *string` — nil for regular blocks. Non-nil = SBP parallel group. All blocks with same GroupKey must share identical (Day, StartSlot).

#### `internal/algorithm/schedule_matrix.go`
The conflict-detection engine.
- `PlaceBlock` — checks `CanPlaceBlock` first, then writes to classGrid and teacherGrid.
- `CanPlaceBlock` — **returns an error if the block is already placed.** This is intentional and critical.
- `RemoveBlock` — removes from both grids and the placement map.
- `MoveBlock` — atomic remove + place with rollback on failure.
Do not change conflict semantics. The SA relies on exact error behavior.

#### `internal/algorithm/matrix_slot.go`
Defines `Slot`, `DaySlots`, `MatrixDays`, and `GenerateSlots()`.
`GenerateSlots()` is called inside `CountSoftViolations` for PJOK deadline checking. It is cached implicitly (called once per fitness evaluation during PJOK check). If you need to change the school's time grid, change it here.
`MatrixDays` defines day order: `["monday","tuesday","wednesday","thursday","friday"]`.

---

### DO NOT TOUCH — Critical invariants

#### The SBP Group Invariant
**All `MatrixBlock` instances sharing a `GroupKey` must always have the same `(Day, StartSlot)` gene, and must always be either all-placed or all-unplaced in the matrix.**

Violating this causes permanent unplaced blocks because:
1. `CanPlaceBlock` returns error for already-placed blocks.
2. `findFreeCandidateForGroup` calls `CanPlaceBlock` for every group member.
3. If some members are placed and some are not, `findFreeCandidateForGroup` fails for every candidate (the placed members reject every slot as "already placed"), and the unplaced members can never be resolved.

This invariant is maintained by:
- GA: `RandomChromosome`, `UniformCrossover`, `applyMutation` all assign the same gene to all group members.
- SA: displace moves skip if any conflict is a group member. Perturbation evicts entire groups. Group unplaced moves place all members at once.
- Decode: `GenerateMatrixBlocks` puts grouped blocks first so they are placed before ungrouped blocks can occupy their slots.

**If you add any new move type in SA, you must maintain this invariant.**

#### Decode Order
`DecodeChromosome` places blocks in array order. The first block to claim a slot wins all conflicts. `GenerateMatrixBlocks` intentionally returns grouped blocks first (`append(grouped, ungrouped...)`). Do not change this ordering.

---

## Known Limitations (Future Work)

1. **SBP groups are frozen in SA once placed.** The swap move skips all group members. If an SBP group lands on a bad day (causing soft violations), SA cannot rebalance it. A group-level swap (swap two entire groups atomically) would fix this. Before implementing, check `sameDaySplitGrouped` in the response — if it is 0, this is not currently causing problems.

2. **Default parameters are not domain-tuned.** `PopulationSize=100`, `Generations=1000`, `MutationRate=0.02` are generic GA defaults. For school timetabling literature suggests larger populations (200–500) and lower mutation rates.

3. **`CountSoftViolations` calls `GenerateSlots()` on every invocation** for the PJOK check. `GenerateSlots()` is cheap but not free. If profiling shows it as a hotspot, cache the slot-end map outside the GA/SA loop.

---

## Current Soft Violation Baseline

With default parameters, a typical run achieves:
- `unplaced: 0` (always achievable)
- `violations: 20–40`
  - `sameDaySplit: 8–12` (regular subjects split on same day)
  - `sameDaySplitGrouped: 0` (SBP groups are not the problem)
  - `pjokAfterDeadline: 4–7` (acceptable — school has indoor facilities)

The violations are from genuine scheduling difficulty, not algorithmic bugs.
Further improvement requires parameter tuning, not structural changes.

---

## API Endpoints

- `POST /v2/schedule/generate` — pure GA only
- `POST /v3/schedule/generate` — GA + SA hybrid (use this one)
- `GET /v3/schedule/readable` — same as above but with human-readable names

All accept query parameters to override defaults: `populationSize`, `generations`, `mutationRate`, `eliteCount`, `saIterations`, `saCoolingRate`, `saInitialTemperature`, `saPerturbCount`, `saPerturbAfter`.

---

## Key Relationships

```
TeachingAssignment (DB)
  → GenerateMatrixBlocks()
  → []MatrixBlock  ← GroupKey links SBP parallel classes
  → BuildCandidateIndex()
  → map[blockID][]Gene  ← valid placements per block

Chromosome (GA search space)
  → []Gene, index i = placement for blocks[i]
  → DecodeChromosome()
  → ScheduleMatrix  ← conflict-checked grid
  → CountSoftViolations()  ← fitness score

SA operates directly on ScheduleMatrix (no chromosome decode)
  → PlaceBlock / RemoveBlock / CanPlaceBlock
  → accepts/rejects moves via Boltzmann criterion
```
