"use client";

import { useMemo, useState } from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { type ScheduleEntry, DAYS, DAY_LABELS, type DayKey } from "@/types";

interface Props {
  entries: ScheduleEntry[];
}

type ViewMode = "all-classes-by-day" | "class-day";

export function ScheduleTable({ entries }: Props) {
  const [viewMode, setViewMode] = useState<ViewMode>("all-classes-by-day");
  const [selectedDay, setSelectedDay] = useState<DayKey>("monday");
  const [selectedClass, setSelectedClass] = useState<string>("");

  const classes = useMemo(
    () => [...new Set(entries.map((e) => e.className))].sort(),
    [entries]
  );

  const timeSlots = useMemo(() => {
    const starts = [...new Set(entries.map((e) => e.timeStart))].sort();
    return starts;
  }, [entries]);

  // ── View: Semua kelas dalam satu hari ─────────────────────────────────────
  const allClassesForDay = useMemo(() => {
    const filtered = entries.filter((e) => e.day === selectedDay);
    // group by timeStart → class → entries
    const bySlot: Record<string, Record<string, ScheduleEntry[]>> = {};
    for (const e of filtered) {
      if (!bySlot[e.timeStart]) bySlot[e.timeStart] = {};
      if (!bySlot[e.timeStart][e.className]) bySlot[e.timeStart][e.className] = [];
      bySlot[e.timeStart][e.className].push(e);
    }
    return bySlot;
  }, [entries, selectedDay]);

  const dayTimeSlots = useMemo(() => {
    return [...new Set(entries.filter((e) => e.day === selectedDay).map((e) => e.timeStart))].sort();
  }, [entries, selectedDay]);

  // ── View: Satu kelas, semua hari ──────────────────────────────────────────
  const classEntries = useMemo(() => {
    if (!selectedClass) return {};
    const filtered = entries.filter((e) => e.className === selectedClass);
    const bySlot: Record<string, Record<DayKey, ScheduleEntry[]>> = {};
    for (const e of filtered) {
      if (!bySlot[e.timeStart]) bySlot[e.timeStart] = {} as Record<DayKey, ScheduleEntry[]>;
      if (!bySlot[e.timeStart][e.day as DayKey]) bySlot[e.timeStart][e.day as DayKey] = [];
      bySlot[e.timeStart][e.day as DayKey].push(e);
    }
    return bySlot;
  }, [entries, selectedClass]);

  return (
    <div className="space-y-4">
      {/* ── Filters ─────────────────────────────────────────────────────────── */}
      <div className="flex flex-wrap gap-3 items-center">
        <Select value={viewMode} onValueChange={(v) => setViewMode(v as ViewMode)}>
          <SelectTrigger className="w-52">
            <SelectValue placeholder="Tampilan" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all-classes-by-day">Semua Kelas per Hari</SelectItem>
            <SelectItem value="class-day">Per Kelas (Semua Hari)</SelectItem>
          </SelectContent>
        </Select>

        {viewMode === "all-classes-by-day" && (
          <Select value={selectedDay} onValueChange={(v) => setSelectedDay(v as DayKey)}>
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DAYS.map((d) => (
                <SelectItem key={d} value={d}>{DAY_LABELS[d]}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        {viewMode === "class-day" && (
          <Select value={selectedClass} onValueChange={(v) => setSelectedClass(v ?? "")}>
            <SelectTrigger className="w-36">
              <SelectValue placeholder="Pilih Kelas" />
            </SelectTrigger>
            <SelectContent>
              {classes.map((c) => (
                <SelectItem key={c} value={c}>{c}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        <span className="text-sm text-gray-500 ml-auto">{entries.length} JP total</span>
      </div>

      {/* ── Table: Semua Kelas per Hari ───────────────────────────────────── */}
      {viewMode === "all-classes-by-day" && (
        <div className="overflow-x-auto rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-28 bg-gray-50">Waktu</TableHead>
                {classes.map((c) => (
                  <TableHead key={c} className="text-center bg-gray-50 min-w-28">{c}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {dayTimeSlots.map((slot) => (
                <TableRow key={slot}>
                  <TableCell className="font-mono text-xs text-gray-500 whitespace-nowrap">
                    {slot}
                  </TableCell>
                  {classes.map((cls) => {
                    const cell = allClassesForDay[slot]?.[cls] ?? [];
                    return (
                      <TableCell key={cls} className="text-center p-1">
                        {cell.map((e, i) => (
                          <div key={i} className="text-xs leading-tight">
                            <div className="font-medium">{e.subjectName}</div>
                            {e.teacherName && (
                              <div className="text-gray-400 truncate max-w-24">{e.teacherName.split(",")[0]}</div>
                            )}
                          </div>
                        ))}
                      </TableCell>
                    );
                  })}
                </TableRow>
              ))}
              {dayTimeSlots.length === 0 && (
                <TableRow>
                  <TableCell colSpan={classes.length + 1} className="text-center text-gray-400 py-8">
                    Tidak ada jadwal pada hari ini
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}

      {/* ── Table: Satu Kelas, Semua Hari ────────────────────────────────── */}
      {viewMode === "class-day" && (
        <div className="overflow-x-auto rounded-md border">
          {!selectedClass ? (
            <div className="p-8 text-center text-gray-400">Pilih kelas untuk melihat jadwal</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-28 bg-gray-50">Waktu</TableHead>
                  {DAYS.map((d) => (
                    <TableHead key={d} className="text-center bg-gray-50 min-w-28">{DAY_LABELS[d]}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {timeSlots
                  .filter((slot) => classEntries[slot])
                  .map((slot) => (
                    <TableRow key={slot}>
                      <TableCell className="font-mono text-xs text-gray-500 whitespace-nowrap">
                        {slot}
                      </TableCell>
                      {DAYS.map((day) => {
                        const cell = classEntries[slot]?.[day] ?? [];
                        return (
                          <TableCell key={day} className="text-center p-1">
                            {cell.map((e, i) => (
                              <div key={i} className="text-xs leading-tight">
                                <div className="font-medium">{e.subjectName}</div>
                                {e.teacherName && (
                                  <div className="text-gray-400 truncate max-w-24">{e.teacherName.split(",")[0]}</div>
                                )}
                              </div>
                            ))}
                          </TableCell>
                        );
                      })}
                    </TableRow>
                  ))}
                {Object.keys(classEntries).length === 0 && (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-gray-400 py-8">
                      Tidak ada jadwal
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          )}
        </div>
      )}
    </div>
  );
}

export function LogSummary({ meta }: { meta: { result: { violations: number; unplaced: number; softBreakdown: { sameDaySplit: number; pjokAfterDeadline: number } }; totalElapsedMs: number; loopCount?: number } }) {
  const { result, totalElapsedMs, loopCount } = meta;
  const elapsedSec = (totalElapsedMs / 1000).toFixed(1);

  return (
    <div className="flex flex-wrap gap-3 text-sm">
      <Badge variant={result.unplaced === 0 ? "default" : "destructive"}>
        Unplaced: {result.unplaced}
      </Badge>
      <Badge variant={result.violations === 0 ? "default" : "secondary"}>
        Violations: {result.violations}
      </Badge>
      <Badge variant="outline">Same-day split: {result.softBreakdown?.sameDaySplit ?? 0}</Badge>
      <Badge variant="outline">PJOK after deadline: {result.softBreakdown?.pjokAfterDeadline ?? 0}</Badge>
      <Badge variant="outline">Waktu: {elapsedSec}s</Badge>
      {loopCount && loopCount > 0 && (
        <Badge variant="outline">Loop: {loopCount}</Badge>
      )}
    </div>
  );
}
