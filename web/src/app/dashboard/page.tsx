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
import { Progress } from "@/components/ui/progress";
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
} from "lucide-react";
import {
  apiGetDataStatus,
  apiUploadData,
  templateDownloadUrl,
  buildGenerateStreamUrl,
  apiSaveSchedule,
} from "@/lib/api";
import type {
  DataStatus,
  ScheduleEntry,
  ScheduleMeta,
  GenerateResult,
} from "@/types";

interface ProgressState {
  phase: "idle" | "ga" | "ts" | "done" | "error";
  gaPercent: number;
  tsPercent: number;
  unplaced: number;
  violations: number;
  message: string;
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
    } catch {
      /* ignore */
    }
  }, [token]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

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
  const esRef = useRef<EventSource | null>(null);

  function startGenerate() {
    if (esRef.current) esRef.current.close();
    setResult(null);
    setProgress({ phase: "ga", gaPercent: 0, tsPercent: 0, unplaced: 0, violations: 0, message: "Memulai GA..." });

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
        message: `GA gen ${d.generation}/${d.totalGenerations} | unplaced: ${d.bestUnplaced}`,
      }));
    });

    es.addEventListener("phase_change", () => {
      setProgress((p) => ({ ...p, phase: "ts", gaPercent: 100, message: "Tabu Search dimulai..." }));
    });

    es.addEventListener("ts_progress", (e) => {
      const d = JSON.parse(e.data);
      setProgress((p) => ({
        ...p,
        phase: "ts",
        tsPercent: d.progressPercent ?? p.tsPercent,
        unplaced: d.bestUnplaced ?? p.unplaced,
        violations: d.bestSoftViolations ?? p.violations,
        message: `TS iter ${d.iteration} | unplaced: ${d.bestUnplaced}`,
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
        message: "Selesai",
      });
      es.close();
    });

    es.addEventListener("error", () => {
      if (es.readyState === EventSource.CLOSED) return;
      setProgress((p) => ({ ...p, phase: "error", message: "Koneksi error" }));
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

  return (
    <div className="min-h-screen bg-gray-50">
      <Nav />

      <div className="max-w-7xl mx-auto px-4 py-6 space-y-6">

        {/* ── Data Management ─────────────────────────────────────────────── */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              Manajemen Data
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Status */}
            {dataStatus && (
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                {[
                  { label: "Guru", value: dataStatus.teachers },
                  { label: "Kelas Aktif", value: dataStatus.activeClasses },
                  { label: "Mata Pelajaran", value: dataStatus.subjects },
                  { label: "Penugasan", value: dataStatus.teachingAssignments },
                ].map((item) => (
                  <div key={item.label} className="border rounded-lg p-3 bg-white text-center">
                    <div className="text-2xl font-bold">{item.value}</div>
                    <div className="text-xs text-gray-500">{item.label}</div>
                  </div>
                ))}
              </div>
            )}

            {/* Actions */}
            <div className="flex flex-wrap gap-3">
              <a
                href={templateDownloadUrl()}
                download="template_data_jadwal.xlsx"
                onClick={(e) => {
                  const url = templateDownloadUrl();
                  if (!url) { e.preventDefault(); return; }
                  fetch(url, { headers: { Authorization: `Bearer ${token}` } })
                    .then((r) => r.blob())
                    .then((b) => {
                      const a = document.createElement("a");
                      a.href = URL.createObjectURL(b);
                      a.download = "template_data_jadwal.xlsx";
                      a.click();
                    });
                  e.preventDefault();
                }}
              >
                <Button variant="outline" size="sm">
                  <Download className="h-4 w-4 mr-1.5" />
                  Download Template Excel
                </Button>
              </a>

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

        {/* ── Generate ────────────────────────────────────────────────────── */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Play className="h-5 w-5" />
              Generate Jadwal
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <Button
              onClick={startGenerate}
              disabled={generating || !dataStatus?.teachingAssignments}
              size="default"
            >
              {generating ? (
                <><Loader2 className="h-4 w-4 mr-2 animate-spin" />Generating...</>
              ) : (
                <><Play className="h-4 w-4 mr-2" />Generate Jadwal</>
              )}
            </Button>

            {/* Progress */}
            {progress.phase !== "idle" && (
              <div className="space-y-3 p-4 border rounded-lg bg-white">
                <div className="flex items-center gap-2">
                  {progress.phase === "done" ? (
                    <CheckCircle2 className="h-4 w-4 text-green-500" />
                  ) : progress.phase === "error" ? (
                    <AlertCircle className="h-4 w-4 text-red-500" />
                  ) : (
                    <Loader2 className="h-4 w-4 animate-spin text-blue-500" />
                  )}
                  <span className="text-sm font-medium">{progress.message}</span>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center gap-2 text-xs text-gray-500">
                    <span className="w-6">GA</span>
                    <Progress value={progress.gaPercent} className="flex-1 h-2" />
                    <span className="w-8 text-right">{progress.gaPercent.toFixed(0)}%</span>
                  </div>
                  <div className="flex items-center gap-2 text-xs text-gray-500">
                    <span className="w-6">TS</span>
                    <Progress value={progress.tsPercent} className="flex-1 h-2" />
                    <span className="w-8 text-right">{progress.tsPercent.toFixed(0)}%</span>
                  </div>
                </div>

                <div className="flex gap-3">
                  <Badge variant={progress.unplaced === 0 ? "default" : "destructive"}>
                    Unplaced: {progress.unplaced}
                  </Badge>
                  <Badge variant="secondary">Violations: {progress.violations}</Badge>
                </div>
              </div>
            )}

            {/* Result summary + save */}
            {result && progress.phase === "done" && (
              <div className="space-y-3 p-4 border rounded-lg bg-white">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-semibold">Hasil Generasi</span>
                  <Button size="sm" onClick={() => setSaveOpen(true)}>
                    <Save className="h-4 w-4 mr-1.5" />
                    Simpan Jadwal
                  </Button>
                </div>
                <LogSummary meta={result.meta} />
              </div>
            )}
          </CardContent>
        </Card>

        {/* ── Schedule table ───────────────────────────────────────────────── */}
        {result && (
          <Card>
            <CardHeader>
              <CardTitle>Jadwal yang Digenerate</CardTitle>
            </CardHeader>
            <CardContent>
              <ScheduleTable entries={result.entries} />
            </CardContent>
          </Card>
        )}
      </div>

      {/* ── Save dialog ───────────────────────────────────────────────────── */}
      <Dialog open={saveOpen} onOpenChange={setSaveOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Simpan Jadwal</DialogTitle>
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
            <Button variant="outline" onClick={() => setSaveOpen(false)}>Batal</Button>
            <Button onClick={handleSave} disabled={!saveTitle.trim() || saving}>
              {saving ? <Loader2 className="h-4 w-4 mr-1.5 animate-spin" /> : null}
              Simpan
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
