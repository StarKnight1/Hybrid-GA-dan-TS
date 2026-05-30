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

  useEffect(() => {
    loadList();
  }, [loadList]);

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
    const url = scheduleExportUrl(id);
    fetch(url, { headers: { Authorization: `Bearer ${token}` } })
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
    <div className="min-h-screen bg-gray-50">
      <Nav />

      <div className="max-w-7xl mx-auto px-4 py-6 space-y-6">

        {/* ── Schedule list ────────────────────────────────────────────────── */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <CalendarDays className="h-5 w-5" />
              Jadwal Tersimpan
            </CardTitle>
          </CardHeader>
          <CardContent>
            {listLoading ? (
              <div className="flex items-center gap-2 text-gray-400 py-6">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span>Memuat...</span>
              </div>
            ) : list.length === 0 ? (
              <div className="text-center py-8 text-gray-400">
                <CalendarDays className="h-10 w-10 mx-auto mb-2 opacity-40" />
                <p>Belum ada jadwal yang diterbitkan</p>
              </div>
            ) : (
              <div className="space-y-2">
                {list.map((item) => (
                  <div
                    key={item.id}
                    className={`flex items-center justify-between p-3 rounded-lg border cursor-pointer transition-colors ${
                      selected?.id === item.id
                        ? "border-blue-500 bg-blue-50"
                        : "border-gray-200 bg-white hover:bg-gray-50"
                    }`}
                    onClick={() => handleSelect(item.id)}
                  >
                    <div className="space-y-0.5">
                      <div className="font-medium text-sm">{item.title}</div>
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
                    <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleExport(item.id, item.title)}
                        title="Download Excel"
                      >
                        <Download className="h-4 w-4" />
                      </Button>
                      {isAdmin && (
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-red-500 hover:text-red-700 hover:bg-red-50"
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

        {/* ── Selected schedule ─────────────────────────────────────────────── */}
        {selectedLoading && (
          <div className="flex items-center gap-2 text-gray-400 py-4">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span>Memuat jadwal...</span>
          </div>
        )}

        {selected && !selectedLoading && (
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between flex-wrap gap-3">
                <CardTitle>{selected.title}</CardTitle>
                <div className="flex items-center gap-2">
                  <Badge variant="outline">{selected.entries.length} JP</Badge>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => handleExport(selected.id, selected.title)}
                  >
                    <Download className="h-4 w-4 mr-1.5" />
                    Download Excel
                  </Button>
                </div>
              </div>
              {selected.meta?.result && (
                <div className="mt-2">
                  <LogSummary meta={selected.meta} />
                </div>
              )}
            </CardHeader>
            <CardContent>
              <ScheduleTable entries={selected.entries} />
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
