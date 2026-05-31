"use client";

import { useState, useEffect, useCallback } from "react";
import { useSession } from "next-auth/react";
import { toast } from "sonner";
import { Nav } from "@/components/nav";
import { ScheduleTable, LogSummary } from "@/components/schedule-table";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Download,
  Trash2,
  CalendarDays,
  Loader2,
  Rocket,
  CheckCircle2,
  Clock,
  BookOpen,
  Users,
} from "lucide-react";
import {
  apiListSchedules,
  apiGetSchedule,
  apiDeleteSchedule,
  apiDeploySchedule,
  apiGetActiveSchedule,
  apiGetMe,
  scheduleExportUrl,
} from "@/lib/api";
import type {
  SavedScheduleListItem,
  SavedSchedule,
  ScheduleEntry,
  UserProfile,
  DayKey,
} from "@/types";
import { DAYS, DAY_LABELS } from "@/types";

// ── Fungsi bantu

function getTodayKey(): DayKey {
  const map: Record<number, DayKey> = {
    1: "monday",
    2: "tuesday",
    3: "wednesday",
    4: "thursday",
    5: "friday",
  };
  return map[new Date().getDay()] ?? "monday";
}

function sortByTime(entries: ScheduleEntry[]) {
  return [...entries].sort((a, b) => a.timeStart.localeCompare(b.timeStart));
}

// ── Komponen jadwal per hari — dipakai siswa dan guru

function DayScheduleView({
  entries,
  labelFn,
  emptyMsg,
  activeScheduleTitle,
}: {
  entries: ScheduleEntry[];
  labelFn: (e: ScheduleEntry) => string;
  emptyMsg: string;
  activeScheduleTitle: string;
}) {
  const [selectedDay, setSelectedDay] = useState<DayKey>(getTodayKey());

  const dayEntries = sortByTime(
    entries.filter((e) => e.day === selectedDay)
  );

  return (
    <div className="space-y-4">
      <p className="text-xs text-gray-400">Jadwal aktif: {activeScheduleTitle}</p>

      {/* Tab hari */}
      <div className="flex gap-1.5 flex-wrap">
        {DAYS.map((d) => {
          const count = entries.filter((e) => e.day === d).length;
          return (
            <button
              key={d}
              onClick={() => setSelectedDay(d)}
              className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                selectedDay === d
                  ? "bg-blue-700 text-white shadow-md"
                  : "bg-white text-blue-700 border border-blue-200 hover:bg-blue-50"
              }`}
            >
              {DAY_LABELS[d]}
              {count > 0 && (
                <span
                  className={`ml-1.5 text-xs ${
                    selectedDay === d ? "text-blue-200" : "text-blue-400"
                  }`}
                >
                  ({count})
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* Daftar pelajaran hari dipilih */}
      {dayEntries.length === 0 ? (
        <div className="text-center py-10 text-gray-400">
          <CalendarDays className="h-10 w-10 mx-auto mb-2 opacity-30" />
          <p>{emptyMsg}</p>
        </div>
      ) : (
        <div className="space-y-2">
          {dayEntries.map((e, i) => (
            <div
              key={i}
              className="flex items-center gap-3 p-3 rounded-xl bg-white border border-blue-100 shadow-sm"
            >
              {/* Waktu */}
              <div className="flex items-center gap-1 text-xs font-mono text-blue-600 bg-blue-50 px-2.5 py-1.5 rounded-lg shrink-0 min-w-[110px] justify-center">
                <Clock className="h-3 w-3" />
                {e.timeStart}–{e.timeEnd}
              </div>

              {/* Mata pelajaran */}
              <div className="flex-1 min-w-0">
                <p className="font-semibold text-blue-900 text-sm truncate">
                  {e.subjectName}
                </p>
                <p className="text-xs text-gray-500 truncate">{labelFn(e)}</p>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ── Tampilan siswa

function StudentView({
  token,
  profile,
}: {
  token: string;
  profile: UserProfile;
}) {
  const [schedule, setSchedule] = useState<SavedSchedule | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    apiGetActiveSchedule(token)
      .then(setSchedule)
      .catch(() => setSchedule(null))
      .finally(() => setLoading(false));
  }, [token]);

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-gray-400 py-16 justify-center">
        <Loader2 className="h-5 w-5 animate-spin" />
        <span>Memuat jadwal...</span>
      </div>
    );
  }

  if (!schedule) {
    return (
      <Card className="border-blue-100">
        <CardContent className="py-16 text-center text-gray-400">
          <CalendarDays className="h-12 w-12 mx-auto mb-3 opacity-30" />
          <p className="font-medium">Belum ada jadwal yang diterbitkan</p>
          <p className="text-xs mt-1">Admin belum menerbitkan jadwal.</p>
        </CardContent>
      </Card>
    );
  }

  const myClassName = profile.className ?? "";
  const myEntries = schedule.entries.filter(
    (e) => e.className === myClassName
  );

  return (
    <Card className="border-blue-100">
      <CardHeader className="pb-3 border-b border-blue-50 bg-gradient-to-r from-blue-700 to-blue-800 rounded-t-xl">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-white/10 rounded-lg">
            <BookOpen className="h-5 w-5 text-white" />
          </div>
          <div>
            <CardTitle className="text-white text-base">
              Jadwal Kelas {myClassName || "—"}
            </CardTitle>
            <p className="text-blue-200 text-xs mt-0.5">{profile.username}</p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-4">
        {myEntries.length === 0 && myClassName ? (
          <div className="text-center py-10 text-gray-400">
            <p>Belum ada jadwal untuk kelas {myClassName}.</p>
          </div>
        ) : (
          <DayScheduleView
            entries={myEntries}
            labelFn={(e) => e.teacherName || "—"}
            emptyMsg={`Tidak ada pelajaran pada hari ini untuk kelas ${myClassName}.`}
            activeScheduleTitle={schedule.title}
          />
        )}
      </CardContent>
    </Card>
  );
}

// ── Tampilan guru

function TeacherView({
  token,
  profile,
}: {
  token: string;
  profile: UserProfile;
}) {
  const [schedule, setSchedule] = useState<SavedSchedule | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    apiGetActiveSchedule(token)
      .then(setSchedule)
      .catch(() => setSchedule(null))
      .finally(() => setLoading(false));
  }, [token]);

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-gray-400 py-16 justify-center">
        <Loader2 className="h-5 w-5 animate-spin" />
        <span>Memuat jadwal...</span>
      </div>
    );
  }

  if (!schedule) {
    return (
      <Card className="border-blue-100">
        <CardContent className="py-16 text-center text-gray-400">
          <CalendarDays className="h-12 w-12 mx-auto mb-3 opacity-30" />
          <p className="font-medium">Belum ada jadwal yang diterbitkan</p>
          <p className="text-xs mt-1">Admin belum menerbitkan jadwal.</p>
        </CardContent>
      </Card>
    );
  }

  // Filter berdasarkan ID guru, fallback ke nama jika ID tidak ada
  const myEntries = schedule.entries.filter((e) =>
    profile.teacherId != null
      ? e.teacherId === profile.teacherId
      : e.teacherName === profile.teacherName
  );

  return (
    <Card className="border-blue-100">
      <CardHeader className="pb-3 border-b border-blue-50 bg-gradient-to-r from-blue-700 to-blue-800 rounded-t-xl">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-white/10 rounded-lg">
            <Users className="h-5 w-5 text-white" />
          </div>
          <div>
            <CardTitle className="text-white text-base">
              Jadwal Mengajar
            </CardTitle>
            <p className="text-blue-200 text-xs mt-0.5">
              {profile.teacherName || profile.username}
            </p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-4">
        <DayScheduleView
          entries={myEntries}
          labelFn={(e) => `Kelas ${e.className}`}
          emptyMsg="Tidak ada jadwal mengajar pada hari ini."
          activeScheduleTitle={schedule.title}
        />
      </CardContent>
    </Card>
  );
}

// ── Tampilan admin

function AdminView({ token }: { token: string }) {
  const [list, setList] = useState<SavedScheduleListItem[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [selected, setSelected] = useState<SavedSchedule | null>(null);
  const [selectedLoading, setSelectedLoading] = useState(false);
  const [deploying, setDeploying] = useState<number | null>(null);

  const loadList = useCallback(async () => {
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

  async function handleDeploy(id: number, title: string) {
    setDeploying(id);
    try {
      await apiDeploySchedule(token, id);
      toast.success(`Jadwal "${title}" berhasil diterbitkan ke siswa & guru`);
      setList((l) =>
        l.map((x) => ({ ...x, isActive: x.id === id }))
      );
    } catch (err) {
      toast.error(`Gagal menerbitkan: ${(err as Error).message}`);
    } finally {
      setDeploying(null);
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
    <div className="space-y-6">
      {/* Daftar jadwal tersimpan */}
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
              <p className="font-medium">Belum ada jadwal yang disimpan</p>
              <p className="text-xs mt-1">
                Gunakan Dashboard untuk generate dan simpan jadwal.
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
                  <div className="space-y-0.5 min-w-0 flex items-start gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="font-semibold text-sm text-blue-900 truncate">
                          {item.title}
                        </span>
                        {item.isActive && (
                          <Badge className="bg-emerald-100 text-emerald-700 border-emerald-200 text-[10px] py-0 px-1.5 flex items-center gap-1">
                            <CheckCircle2 className="h-2.5 w-2.5" />
                            Diterbitkan
                          </Badge>
                        )}
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
                  </div>

                  <div
                    className="flex items-center gap-1.5 shrink-0 ml-3"
                    onClick={(e) => e.stopPropagation()}
                  >
                    {/* Tombol terbitkan */}
                    {item.isActive ? (
                      <span className="text-xs text-emerald-600 font-medium px-2">
                        Aktif
                      </span>
                    ) : (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-amber-500 hover:text-amber-700 hover:bg-amber-50 h-8 w-8 p-0"
                        onClick={() => handleDeploy(item.id, item.title)}
                        disabled={deploying === item.id}
                        title="Terbitkan ke siswa & guru"
                      >
                        {deploying === item.id ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <Rocket className="h-4 w-4" />
                        )}
                      </Button>
                    )}

                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-blue-500 hover:text-blue-700 hover:bg-blue-100 h-8 w-8 p-0"
                      onClick={() => handleExport(item.id, item.title)}
                      title="Download Excel"
                    >
                      <Download className="h-4 w-4" />
                    </Button>

                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-red-400 hover:text-red-600 hover:bg-red-50 h-8 w-8 p-0"
                      onClick={() => handleDelete(item.id, item.title)}
                      title="Hapus"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Loading jadwal dipilih */}
      {selectedLoading && (
        <div className="flex items-center gap-2 text-gray-400 py-4 justify-center">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Memuat jadwal...</span>
        </div>
      )}

      {/* Detail jadwal dipilih */}
      {selected && !selectedLoading && (
        <Card className="border-blue-100">
          <CardHeader className="pb-3 border-b border-blue-50">
            <div className="flex items-center justify-between flex-wrap gap-3">
              <div>
                <CardTitle className="text-blue-800 text-base">
                  {selected.title}
                </CardTitle>
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
  );
}

// ── Halaman utama

export default function JadwalPage() {
  const { data: session } = useSession();
  const token = session?.accessToken ?? "";
  const role = session?.role ?? "";

  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [profileLoading, setProfileLoading] = useState(true);

  useEffect(() => {
    if (!token) return;
    apiGetMe(token)
      .then(setProfile)
      .catch(() => setProfile(null))
      .finally(() => setProfileLoading(false));
  }, [token]);

  if (!token || profileLoading) {
    return (
      <div className="min-h-screen bg-slate-50">
        <Nav />
        <div className="flex items-center justify-center py-24 text-gray-400 gap-2">
          <Loader2 className="h-5 w-5 animate-spin" />
          <span>Memuat...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-50">
      <Nav />
      <div className="max-w-7xl mx-auto px-4 py-6">
        {role === "admin" ? (
          <AdminView token={token} />
        ) : role === "student" ? (
          <StudentView token={token} profile={profile!} />
        ) : role === "teacher" ? (
          <TeacherView token={token} profile={profile!} />
        ) : null}
      </div>
    </div>
  );
}
