"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { useSession } from "next-auth/react";
import { toast } from "sonner";
import { Nav } from "@/components/nav";
import { ScheduleTable, LogSummary } from "@/components/schedule-table";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Download,
  Upload,
  Play,
  Save,
  Loader2,
  CheckCircle2,
  AlertCircle,
  Database,
  Users,
  BookOpen,
  GraduationCap,
  ClipboardList,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import {
  apiGetDataStatus,
  apiUploadData,
  templateDownloadUrl,
  buildGenerateStreamUrl,
  apiSaveSchedule,
} from "@/lib/api";
import type { DataStatus, GenerateResult } from "@/types";

interface ProgressState {
  phase: "idle" | "ga" | "ts" | "done" | "error";
  gaPercent: number;
  tsPercent: number;
  unplaced: number;
  violations: number;
  message: string;
}

interface GAFinal {
  generations: number;
  unplaced: number;
  softViolations: number;
  elapsedMs: number;
}

interface TSDetail {
  iteration: number;
  totalIterations: number;
  bestUnplaced: number;
  bestSoftViolations: number;
  elapsedMs: number;
}

export default function DashboardPage() {
  const { data: session } = useSession();
  const token = session?.accessToken ?? "";

  // ── Data status ───────────────────────────────────────────────────────────
  const [dataStatus, setDataStatus] = useState<DataStatus | null>(null);
  const [uploadLoading, setUploadLoading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const fetchStatus = useCallback(async () => {
    if (!token) return;
    try {
      const s = await apiGetDataStatus(token);
      setDataStatus(s);
    } catch { /* ignore */ }
  }, [token]);

  useEffect(() => { fetchStatus(); }, [fetchStatus]);

  async function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploadLoading(true);
    try {
      const res = await apiUploadData(token, file);
      const d = res.data ?? res;
      toast.success(
        `Upload berhasil: ${d.teachers} guru, ${d.classes} kelas, ${d.assignments} penugasan`
      );
      await fetchStatus();
    } catch (err) {
      toast.error(`Upload gagal: ${(err as Error).message}`);
    } finally {
      setUploadLoading(false);
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  }

  // ── Generate ───────────────────────────────────────────────────────────────
  const [progress, setProgress] = useState<ProgressState>({
    phase: "idle",
    gaPercent: 0,
    tsPercent: 0,
    unplaced: 0,
    violations: 0,
    message: "",
  });
  const [result, setResult] = useState<GenerateResult | null>(null);
  const [gaFinal, setGAFinal] = useState<GAFinal | null>(null);
  const [tsDetail, setTSDetail] = useState<TSDetail | null>(null);
  const [showDetailLog, setShowDetailLog] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  function startGenerate() {
    if (esRef.current) esRef.current.close();
    setResult(null);
    setGAFinal(null);
    setTSDetail(null);
    setShowDetailLog(false);
    setProgress({
      phase: "ga",
      gaPercent: 0,
      tsPercent: 0,
      unplaced: 0,
      violations: 0,
      message: "Memulai Genetic Algorithm...",
    });

    const url = buildGenerateStreamUrl(token, { loopUntilFeasible: "true", maxLoops: "5" });
    const es = new EventSource(url);
    esRef.current = es;

    es.addEventListener("ga_progress", (e) => {
      const d = JSON.parse(e.data);
      setProgress((p) => ({
        ...p,
        phase: "ga",
        gaPercent: d.progressPercent ?? p.gaPercent,
        unplaced: d.bestUnplaced ?? p.unplaced,
        violations: d.bestViolations ?? p.violations,
        message: `GA — gen ${d.generation}/${d.totalGenerations} | unplaced: ${d.bestUnplaced}`,
      }));
    });

    es.addEventListener("phase_change", (e) => {
      const d = JSON.parse(e.data);
      if (d.gaResult) {
        setGAFinal({
          generations: d.gaResult.generations,
          unplaced: d.gaResult.unplaced,
          softViolations: d.gaResult.softViolations,
          elapsedMs: d.gaResult.elapsedMs,
        });
      }
      setProgress((p) => ({
        ...p,
        phase: "ts",
        gaPercent: 100,
        message: "Tabu Search dimulai...",
      }));
    });

    es.addEventListener("ts_progress", (e) => {
      const d = JSON.parse(e.data);
      setTSDetail({
        iteration: d.iteration,
        totalIterations: d.totalIterations,
        bestUnplaced: d.bestUnplaced,
        bestSoftViolations: d.bestSoftViolations,
        elapsedMs: d.elapsedMs,
      });
      setProgress((p) => ({
        ...p,
        phase: "ts",
        tsPercent: d.progressPercent ?? p.tsPercent,
        unplaced: d.bestUnplaced ?? p.unplaced,
        violations: d.bestSoftViolations ?? p.violations,
        message: `TS — iter ${d.iteration}/${d.totalIterations} | unplaced: ${d.bestUnplaced}`,
      }));
    });

    es.addEventListener("completed", (e) => {
      const d: GenerateResult = JSON.parse(e.data);
      setResult(d);
      setProgress({
        phase: "done",
        gaPercent: 100,
        tsPercent: 100,
        unplaced: d.meta?.result?.unplaced ?? 0,
        violations: d.meta?.result?.violations ?? 0,
        message: "Selesai!",
      });
      es.close();
    });

    es.addEventListener("error", () => {
      if (es.readyState === EventSource.CLOSED) return;
      setProgress((p) => ({ ...p, phase: "error", message: "Koneksi ke server terputus" }));
      toast.error("Koneksi ke server terputus");
      es.close();
    });
  }

  // ── Save schedule ─────────────────────────────────────────────────────────
  const [saveOpen, setSaveOpen] = useState(false);
  const [saveTitle, setSaveTitle] = useState("");
  const [saving, setSaving] = useState(false);

  async function handleSave() {
    if (!result || !saveTitle.trim()) return;
    setSaving(true);
    try {
      const saved = await apiSaveSchedule(token, saveTitle.trim(), result.entries, result.meta);
      toast.success(`Jadwal "${saveTitle}" tersimpan (ID: ${saved.id})`);
      setSaveOpen(false);
      setSaveTitle("");
    } catch (err) {
      toast.error(`Gagal menyimpan: ${(err as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  const generating = progress.phase === "ga" || progress.phase === "ts";
  const gaComplete = ["ts", "done"].includes(progress.phase);
  const tsComplete = progress.phase === "done";

  return (
    <div className="min-h-screen bg-slate-50">
      <Nav />

      <div className="max-w-7xl mx-auto px-4 py-6 space-y-6">

        {/* ── Data Management ─────────────────────────────────────────────── */}
        <Card className="border-blue-100">
          <CardHeader className="pb-3 border-b border-blue-50">
            <CardTitle className="flex items-center gap-2 text-blue-800 text-base">
              <Database className="h-5 w-5 text-blue-500" />
              Manajemen Data
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 pt-4">
            {dataStatus && (
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                {[
                  { label: "Guru", value: dataStatus.teachers, Icon: Users, bg: "from-blue-500 to-blue-700" },
                  { label: "Kelas Aktif", value: dataStatus.activeClasses, Icon: GraduationCap, bg: "from-blue-600 to-blue-800" },
                  { label: "Mata Pelajaran", value: dataStatus.subjects, Icon: BookOpen, bg: "from-amber-400 to-amber-600" },
                  { label: "Penugasan", value: dataStatus.teachingAssignments, Icon: ClipboardList, bg: "from-amber-500 to-amber-700" },
                ].map(({ label, value, Icon, bg }) => (
                  <div
                    key={label}
                    className={`bg-gradient-to-br ${bg} text-white rounded-xl p-4 text-center shadow-sm`}
                  >
                    <Icon className="h-5 w-5 mx-auto mb-1 opacity-80" />
                    <div className="text-2xl font-bold">{value}</div>
                    <div className="text-xs opacity-80 mt-0.5">{label}</div>
                  </div>
                ))}
              </div>
            )}

            <div className="flex flex-wrap gap-3">
              <Button
                variant="outline"
                size="sm"
                className="border-blue-200 text-blue-700 hover:bg-blue-50"
                onClick={() => {
                  fetch(templateDownloadUrl(), {
                    headers: { Authorization: `Bearer ${token}` },
                  })
                    .then((r) => r.blob())
                    .then((b) => {
                      const a = document.createElement("a");
                      a.href = URL.createObjectURL(b);
                      a.download = "template_data_jadwal.xlsx";
                      a.click();
                    })
                    .catch(() => toast.error("Gagal mengunduh template"));
                }}
              >
                <Download className="h-4 w-4 mr-1.5" />
                Download Template Excel
              </Button>

              <div>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".xlsx,.xls"
                  className="hidden"
                  onChange={handleUpload}
                />
                <Button
                  variant="outline"
                  size="sm"
                  className="border-amber-300 text-amber-700 hover:bg-amber-50"
                  disabled={uploadLoading}
                  onClick={() => fileInputRef.current?.click()}
                >
                  {uploadLoading ? (
                    <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
                  ) : (
                    <Upload className="h-4 w-4 mr-1.5" />
                  )}
                  Upload Data Excel
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* ── Generate Jadwal ──────────────────────────────────────────────── */}
        <Card className="border-blue-100 overflow-hidden">
          <CardHeader className="bg-gradient-to-r from-blue-800 to-blue-600 py-4 rounded-t-xl">
            <CardTitle className="flex items-center gap-2 text-white text-base">
              <Play className="h-5 w-5" />
              Generate Jadwal
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-5 pt-5">

            {/* Phase stepper */}
            <div className="flex items-center gap-2 text-sm select-none">
              {/* GA step */}
              <div className="flex items-center gap-1.5">
                <div className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold border-2 transition-all duration-500 ${
                  progress.phase === "ga"
                    ? "bg-blue-600 border-blue-600 text-white ring-4 ring-blue-100"
                    : gaComplete
                    ? "bg-blue-600 border-blue-600 text-white"
                    : "bg-white border-gray-300 text-gray-400"
                }`}>
                  {gaComplete ? <CheckCircle2 className="h-4 w-4" /> : "1"}
                </div>
                <span className={`font-semibold text-xs ${
                  progress.phase === "ga" ? "text-blue-700" : gaComplete ? "text-blue-600" : "text-gray-400"
                }`}>GA</span>
              </div>

              <div className="flex-1 h-1 bg-gray-200 rounded-full max-w-16 overflow-hidden">
                <div className={`h-full bg-blue-500 rounded-full transition-all duration-700 ${gaComplete ? "w-full" : "w-0"}`} />
              </div>

              {/* TS step */}
              <div className="flex items-center gap-1.5">
                <div className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold border-2 transition-all duration-500 ${
                  progress.phase === "ts"
                    ? "bg-amber-500 border-amber-500 text-white ring-4 ring-amber-100"
                    : tsComplete
                    ? "bg-amber-500 border-amber-500 text-white"
                    : "bg-white border-gray-300 text-gray-400"
                }`}>
                  {tsComplete ? <CheckCircle2 className="h-4 w-4" /> : "2"}
                </div>
                <span className={`font-semibold text-xs ${
                  progress.phase === "ts" ? "text-amber-700" : tsComplete ? "text-amber-600" : "text-gray-400"
                }`}>TS</span>
              </div>

              <div className="flex-1 h-1 bg-gray-200 rounded-full max-w-16 overflow-hidden">
                <div className={`h-full bg-amber-500 rounded-full transition-all duration-700 ${tsComplete ? "w-full" : "w-0"}`} />
              </div>

              {/* Done step */}
              <div className="flex items-center gap-1.5">
                <div className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold border-2 transition-all duration-500 ${
                  tsComplete
                    ? "bg-green-500 border-green-500 text-white ring-4 ring-green-100"
                    : "bg-white border-gray-300 text-gray-400"
                }`}>
                  {tsComplete ? <CheckCircle2 className="h-4 w-4" /> : "3"}
                </div>
                <span className={`font-semibold text-xs ${tsComplete ? "text-green-600" : "text-gray-400"}`}>
                  Selesai
                </span>
              </div>
            </div>

            {/* Generate button */}
            <Button
              onClick={startGenerate}
              disabled={generating || !dataStatus?.teachingAssignments}
              className="bg-blue-600 hover:bg-blue-700 text-white font-semibold px-6"
            >
              {generating ? (
                <><Loader2 className="h-4 w-4 mr-2 animate-spin" />Generating...</>
              ) : (
                <><Play className="h-4 w-4 mr-2" />Generate Jadwal</>
              )}
            </Button>

            {/* Progress display */}
            {progress.phase !== "idle" && (
              <div className="space-y-4">
                {/* Status pill */}
                <div className={`inline-flex items-center gap-2 px-4 py-2 rounded-full text-sm font-medium ${
                  progress.phase === "done"
                    ? "bg-green-100 text-green-700 border border-green-200"
                    : progress.phase === "error"
                    ? "bg-red-100 text-red-700 border border-red-200"
                    : progress.phase === "ts"
                    ? "bg-amber-50 text-amber-700 border border-amber-200"
                    : "bg-blue-50 text-blue-700 border border-blue-200"
                }`}>
                  {progress.phase === "done" ? (
                    <CheckCircle2 className="h-4 w-4" />
                  ) : progress.phase === "error" ? (
                    <AlertCircle className="h-4 w-4" />
                  ) : (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  )}
                  {progress.message}
                </div>

                {/* Progress bars */}
                <div className="space-y-3 bg-gray-50 rounded-xl p-4 border border-gray-100">
                  <div className="flex items-center gap-3">
                    <span className="text-xs font-bold text-blue-600 w-6">GA</span>
                    <div className="flex-1 bg-gray-200 rounded-full h-3 overflow-hidden">
                      <div
                        className="h-full bg-gradient-to-r from-blue-400 to-blue-600 rounded-full transition-all duration-300"
                        style={{ width: `${progress.gaPercent}%` }}
                      />
                    </div>
                    <span className="text-xs text-gray-500 w-10 text-right font-mono">
                      {progress.gaPercent.toFixed(0)}%
                    </span>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className="text-xs font-bold text-amber-600 w-6">TS</span>
                    <div className="flex-1 bg-gray-200 rounded-full h-3 overflow-hidden">
                      <div
                        className="h-full bg-gradient-to-r from-amber-400 to-amber-600 rounded-full transition-all duration-300"
                        style={{ width: `${progress.tsPercent}%` }}
                      />
                    </div>
                    <span className="text-xs text-gray-500 w-10 text-right font-mono">
                      {progress.tsPercent.toFixed(0)}%
                    </span>
                  </div>
                </div>

                {/* Live badges */}
                <div className="flex gap-2 flex-wrap">
                  <Badge
                    className={
                      progress.unplaced === 0
                        ? "bg-green-100 text-green-700 border border-green-200 hover:bg-green-100"
                        : "bg-red-100 text-red-700 border border-red-200 hover:bg-red-100"
                    }
                  >
                    Unplaced: {progress.unplaced}
                  </Badge>
                  <Badge className="bg-amber-100 text-amber-700 border border-amber-200 hover:bg-amber-100">
                    Violations: {progress.violations}
                  </Badge>
                </div>
              </div>
            )}

            {/* Result + save + detail log */}
            {result && progress.phase === "done" && (
              <div className="border border-blue-100 rounded-xl bg-gradient-to-br from-blue-50 to-white p-5 space-y-4">
                <div className="flex items-center justify-between flex-wrap gap-3">
                  <div className="flex items-center gap-2">
                    <CheckCircle2 className="h-5 w-5 text-green-500" />
                    <span className="font-semibold text-blue-900">
                      {result.entries.length} JP berhasil digenerate
                    </span>
                  </div>
                  <Button
                    size="sm"
                    className="bg-blue-600 hover:bg-blue-700 text-white"
                    onClick={() => setSaveOpen(true)}
                  >
                    <Save className="h-4 w-4 mr-1.5" />
                    Simpan Jadwal
                  </Button>
                </div>

                <LogSummary meta={result.meta} />

                {/* Detail log toggle */}
                <button
                  type="button"
                  className="flex items-center gap-1.5 text-xs text-blue-600 hover:text-blue-800 font-medium transition-colors mt-1"
                  onClick={() => setShowDetailLog((v) => !v)}
                >
                  {showDetailLog ? (
                    <ChevronUp className="h-3.5 w-3.5" />
                  ) : (
                    <ChevronDown className="h-3.5 w-3.5" />
                  )}
                  {showDetailLog ? "Sembunyikan Detail Log" : "Lihat Detail Log Algoritma"}
                </button>

                {/* Collapsible detail log */}
                {showDetailLog && (
                  <div className="border border-blue-200 rounded-xl bg-white p-4 space-y-5 text-sm">

                    {/* GA Phase */}
                    {gaFinal && (
                      <div>
                        <div className="flex items-center gap-2 mb-3">
                          <div className="w-3 h-3 rounded-full bg-blue-500 shrink-0" />
                          <span className="font-semibold text-blue-700">Fase GA (Genetic Algorithm)</span>
                          <span className="text-gray-400 text-xs ml-1">
                            {gaFinal.generations} generasi · {(gaFinal.elapsedMs / 1000).toFixed(1)}s
                          </span>
                        </div>
                        <div className="grid grid-cols-3 gap-2 pl-5">
                          {[
                            { label: "Generasi", value: gaFinal.generations, color: "blue" },
                            { label: "Best unplaced", value: gaFinal.unplaced, color: gaFinal.unplaced === 0 ? "green" : "red" },
                            { label: "Soft violations", value: gaFinal.softViolations, color: "blue" },
                          ].map(({ label, value, color }) => (
                            <div key={label} className="bg-blue-50 rounded-lg p-2.5 text-center">
                              <div className={`text-lg font-bold text-${color}-600`}>{value}</div>
                              <div className="text-xs text-gray-500 mt-0.5">{label}</div>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    {/* TS Phase */}
                    {tsDetail && (
                      <div>
                        <div className="flex items-center gap-2 mb-3">
                          <div className="w-3 h-3 rounded-full bg-amber-500 shrink-0" />
                          <span className="font-semibold text-amber-700">Fase TS (Tabu Search)</span>
                          <span className="text-gray-400 text-xs ml-1">
                            {tsDetail.totalIterations} iterasi · {(tsDetail.elapsedMs / 1000).toFixed(1)}s
                          </span>
                        </div>
                        <div className="grid grid-cols-3 gap-2 pl-5">
                          {[
                            { label: "Iterasi akhir", value: tsDetail.iteration, color: "amber" },
                            { label: "Final unplaced", value: tsDetail.bestUnplaced, color: tsDetail.bestUnplaced === 0 ? "green" : "red" },
                            { label: "Soft violations", value: tsDetail.bestSoftViolations, color: "amber" },
                          ].map(({ label, value, color }) => (
                            <div key={label} className="bg-amber-50 rounded-lg p-2.5 text-center">
                              <div className={`text-lg font-bold text-${color}-600`}>{value}</div>
                              <div className="text-xs text-gray-500 mt-0.5">{label}</div>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    {/* Final result breakdown */}
                    <div>
                      <div className="flex items-center gap-2 mb-3">
                        <div className="w-3 h-3 rounded-full bg-green-500 shrink-0" />
                        <span className="font-semibold text-green-700">Hasil Akhir</span>
                        {result.meta.loopCount && result.meta.loopCount > 0 && (
                          <span className="text-gray-400 text-xs ml-1">{result.meta.loopCount}× loop</span>
                        )}
                      </div>
                      <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 pl-5">
                        {[
                          { label: "Total JP", value: result.meta.result.entriesGenerated },
                          { label: "Unplaced", value: result.meta.result.unplaced, warn: result.meta.result.unplaced > 0 },
                          { label: "Hard violations", value: result.meta.result.violations, warn: result.meta.result.violations > 0 },
                          { label: "Same-day split", value: result.meta.result.softBreakdown?.sameDaySplit ?? 0 },
                          { label: "Split (grouped)", value: result.meta.result.softBreakdown?.sameDaySplitGrouped ?? 0 },
                          { label: "PJOK terlambat", value: result.meta.result.softBreakdown?.pjokAfterDeadline ?? 0 },
                          { label: "Total waktu", value: `${(result.meta.totalElapsedMs / 1000).toFixed(1)}s` },
                        ].map(({ label, value, warn }) => (
                          <div key={label} className="bg-green-50 rounded-lg p-2.5 text-center">
                            <div className={`text-lg font-bold ${warn ? "text-red-600" : "text-green-700"}`}>
                              {value}
                            </div>
                            <div className="text-xs text-gray-500 mt-0.5">{label}</div>
                          </div>
                        ))}
                      </div>
                    </div>

                    {/* Input stats */}
                    {result.meta.input && (
                      <div>
                        <div className="flex items-center gap-2 mb-3">
                          <div className="w-3 h-3 rounded-full bg-gray-400 shrink-0" />
                          <span className="font-semibold text-gray-600">Data Input</span>
                        </div>
                        <div className="grid grid-cols-3 gap-2 pl-5">
                          {[
                            { label: "Guru", value: result.meta.input.teachers },
                            { label: "Kelas aktif", value: result.meta.input.activeClasses },
                            { label: "Penugasan", value: result.meta.input.activeAssignments },
                          ].map(({ label, value }) => (
                            <div key={label} className="bg-gray-50 rounded-lg p-2.5 text-center">
                              <div className="text-lg font-bold text-gray-700">{value}</div>
                              <div className="text-xs text-gray-500 mt-0.5">{label}</div>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}
          </CardContent>
        </Card>

        {/* ── Schedule table ───────────────────────────────────────────────── */}
        {result && (
          <Card className="border-blue-100">
            <CardHeader className="pb-3 border-b border-blue-50">
              <CardTitle className="text-blue-800 text-base">Jadwal yang Digenerate</CardTitle>
            </CardHeader>
            <CardContent className="pt-4">
              <ScheduleTable entries={result.entries} />
            </CardContent>
          </Card>
        )}
      </div>

      {/* ── Save dialog ─────────────────────────────────────────────────────── */}
      <Dialog open={saveOpen} onOpenChange={setSaveOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-blue-800">Simpan Jadwal</DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <Label htmlFor="save-title">Judul / Tahun Ajar</Label>
            <Input
              id="save-title"
              placeholder="contoh: Tahun Ajar 2025/2026 Semester 1"
              value={saveTitle}
              onChange={(e) => setSaveTitle(e.target.value)}
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setSaveOpen(false)}>
              Batal
            </Button>
            <Button
              className="bg-blue-600 hover:bg-blue-700 text-white"
              onClick={handleSave}
              disabled={!saveTitle.trim() || saving}
            >
              {saving && <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />}
              Simpan
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
