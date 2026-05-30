"use client";

import { useState, useEffect, useCallback } from "react";
import { useSession } from "next-auth/react";
import { toast } from "sonner";
import { Nav } from "@/components/nav";
import { ScheduleTable, LogSummary } from "@/components/schedule-table";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Download, Trash2, CalendarDays, Loader2 } from "lucide-react";
import {
  apiListSchedules,
  apiGetSchedule,
  apiDeleteSchedule,
  scheduleExportUrl,
} from "@/lib/api";
import type { SavedScheduleListItem, SavedSchedule } from "@/types";

export default function JadwalPage() {
  const { data: session } = useSession();
  const token = session?.accessToken ?? "";
  const isAdmin = session?.role === "admin";

  const [list, setList] = useState<SavedScheduleListItem[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [selected, setSelected] = useState<SavedSchedule | null>(null);
  const [selectedLoading, setSelectedLoading] = useState(false);

  const loadList = useCallback(async () => {
    if (!token) return;
    setListLoading(true);
    try {
      const data = await apiListSchedules(token);
      setList(data ?? []);
    } catch (err) {
      toast.error(`Gagal memuat daftar jadwal: ${(err as Error).message}`);
    } finally {
      setListLoading(false);
    }
  }, [token]);

  useEffect(() => { loadList(); }, [loadList]);

  async function handleSelect(id: number) {
    if (selected?.id === id) return;
    setSelectedLoading(true);
    try {
      const s = await apiGetSchedule(token, id);
      setSelected(s);
    } catch (err) {
      toast.error(`Gagal memuat jadwal: ${(err as Error).message}`);
    } finally {
      setSelectedLoading(false);
    }
  }

  async function handleDelete(id: number, title: string) {
    if (!confirm(`Hapus jadwal "${title}"?`)) return;
    try {
      await apiDeleteSchedule(token, id);
      toast.success("Jadwal dihapus");
      setList((l) => l.filter((x) => x.id !== id));
      if (selected?.id === id) setSelected(null);
    } catch (err) {
      toast.error(`Gagal menghapus: ${(err as Error).message}`);
    }
  }

  function handleExport(id: number, title: string) {
    fetch(scheduleExportUrl(id), {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((r) => r.blob())
      .then((b) => {
        const a = document.createElement("a");
        a.href = URL.createObjectURL(b);
        a.download = title.replace(/\s+/g, "_") + ".xlsx";
        a.click();
      })
      .catch(() => toast.error("Gagal mengunduh"));
  }

  return (
    <div className="min-h-screen bg-slate-50">
      <Nav />

      <div className="max-w-7xl mx-auto px-4 py-6 space-y-6">

        {/* ── Schedule list ─────────────────────────────────────────────────── */}
        <Card className="border-blue-100">
          <CardHeader className="pb-3 border-b border-blue-50">
            <CardTitle className="flex items-center gap-2 text-blue-800 text-base">
              <CalendarDays className="h-5 w-5 text-blue-500" />
              Jadwal Tersimpan
            </CardTitle>
          </CardHeader>
          <CardContent className="pt-4">
            {listLoading ? (
              <div className="flex items-center gap-2 text-gray-400 py-8 justify-center">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span>Memuat...</span>
              </div>
            ) : list.length === 0 ? (
              <div className="text-center py-10 text-gray-400">
                <CalendarDays className="h-12 w-12 mx-auto mb-3 opacity-30" />
                <p className="font-medium">Belum ada jadwal yang diterbitkan</p>
                <p className="text-xs mt-1">
                  {isAdmin ? "Gunakan Dashboard untuk generate dan simpan jadwal." : "Admin belum menerbitkan jadwal."}
                </p>
              </div>
            ) : (
              <div className="space-y-2">
                {list.map((item) => (
                  <div
                    key={item.id}
                    className={`flex items-center justify-between p-3 rounded-xl border cursor-pointer transition-all ${
                      selected?.id === item.id
                        ? "border-blue-400 bg-blue-50 shadow-sm"
                        : "border-gray-200 bg-white hover:bg-blue-50 hover:border-blue-200"
                    }`}
                    onClick={() => handleSelect(item.id)}
                  >
                    <div className="space-y-0.5 min-w-0">
                      <div className="font-semibold text-sm text-blue-900 truncate">
                        {item.title}
                      </div>
                      <div className="text-xs text-gray-400">
                        {new Date(item.createdAt).toLocaleDateString("id-ID", {
                          day: "numeric",
                          month: "long",
                          year: "numeric",
                          hour: "2-digit",
                          minute: "2-digit",
                        })}
                      </div>
                    </div>
                    <div
                      className="flex items-center gap-1.5 shrink-0 ml-3"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-blue-500 hover:text-blue-700 hover:bg-blue-100 h-8 w-8 p-0"
                        onClick={() => handleExport(item.id, item.title)}
                        title="Download Excel"
                      >
                        <Download className="h-4 w-4" />
                      </Button>
                      {isAdmin && (
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-red-400 hover:text-red-600 hover:bg-red-50 h-8 w-8 p-0"
                          onClick={() => handleDelete(item.id, item.title)}
                          title="Hapus"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Loading selected */}
        {selectedLoading && (
          <div className="flex items-center gap-2 text-gray-400 py-4 justify-center">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span>Memuat jadwal...</span>
          </div>
        )}

        {/* Selected schedule */}
        {selected && !selectedLoading && (
          <Card className="border-blue-100">
            <CardHeader className="pb-3 border-b border-blue-50">
              <div className="flex items-center justify-between flex-wrap gap-3">
                <div>
                  <CardTitle className="text-blue-800 text-base">{selected.title}</CardTitle>
                  {selected.meta?.result && (
                    <div className="mt-2">
                      <LogSummary meta={selected.meta} />
                    </div>
                  )}
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Badge
                    variant="outline"
                    className="border-blue-200 text-blue-700 font-semibold"
                  >
                    {selected.entries.length} JP
                  </Badge>
                  <Button
                    size="sm"
                    className="bg-blue-600 hover:bg-blue-700 text-white"
                    onClick={() => handleExport(selected.id, selected.title)}
                  >
                    <Download className="h-4 w-4 mr-1.5" />
                    Download Excel
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="pt-4">
              <ScheduleTable entries={selected.entries} />
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
