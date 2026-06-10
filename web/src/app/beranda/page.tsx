"use client";

import { useState, useEffect } from "react";
import { useSession } from "next-auth/react";
import { useRouter } from "next/navigation";
import { Nav } from "@/components/nav";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { apiGetActiveSchedule } from "@/lib/api";
import { DAYS, DAY_LABELS } from "@/types";
import type { SavedSchedule, ScheduleEntry, DayKey } from "@/types";
import { Calendar, Cpu, Zap, Clock, Loader2 } from "lucide-react";

export default function BerandaPage() {
  const { data: session, status } = useSession();
  const router = useRouter();
  const token = session?.accessToken ?? "";

  const [schedule, setSchedule] = useState<SavedSchedule | null>(null);
  const [loading, setLoading] = useState(true);
  const [activeDay, setActiveDay] = useState<DayKey>("monday");

  useEffect(() => {
    if (status === "loading") return;
    if (!session) { router.replace("/login"); return; }
    if (session.role !== "admin") { router.replace("/jadwal"); return; }
  }, [session, status, router]);

  useEffect(() => {
    if (!token) return;
    apiGetActiveSchedule(token)
      .then(setSchedule)
      .catch(() => setSchedule(null))
      .finally(() => setLoading(false));
  }, [token]);

  const dayEntries: ScheduleEntry[] = schedule
    ? schedule.entries
        .filter((e) => e.day === activeDay)
        .sort((a, b) => {
          const t = a.timeStart.localeCompare(b.timeStart);
          return t !== 0 ? t : a.className.localeCompare(b.className);
        })
    : [];

  // Timetable grid: rows = time slots, columns = class names
  const timeSlots = [...new Set(dayEntries.map((e) => `${e.timeStart}–${e.timeEnd}`))].sort();
  const classNames = [...new Set(dayEntries.map((e) => e.className))].sort();
  const grid: Record<string, Record<string, ScheduleEntry>> = {};
  for (const e of dayEntries) {
    const slot = `${e.timeStart}–${e.timeEnd}`;
    if (!grid[slot]) grid[slot] = {};
    grid[slot][e.className] = e;
  }

  return (
    <div className="min-h-screen flex flex-col bg-gray-50">
      <Nav />

      {/* Hero */}
      <div className="bg-gradient-to-br from-blue-900 via-blue-800 to-blue-700 text-white py-14 px-4">
        <div className="max-w-5xl mx-auto text-center">
          <h1 className="text-4xl font-bold mb-3 tracking-tight">
            Sistem Penjadwalan Pelajaran
          </h1>
          <p className="text-blue-200 text-lg max-w-2xl mx-auto leading-relaxed">
            SMP Mater Dei &mdash; penjadwalan pelajaran berbasis algoritma hibrid{" "}
            <span className="text-amber-300 font-semibold">Genetic Algorithm</span> dan{" "}
            <span className="text-amber-300 font-semibold">Tabu Search</span>
          </p>
        </div>
      </div>

      <div className="max-w-7xl mx-auto px-4 py-10 w-full flex flex-col gap-12">

        {/* Active Schedule */}
        <section>
          <h2 className="text-xl font-semibold mb-4 text-gray-800 flex items-center gap-2">
            <Calendar className="h-5 w-5 text-blue-600" />
            Jadwal Aktif
          </h2>

          {loading ? (
            <Card>
              <CardContent className="py-10 flex items-center justify-center gap-2 text-gray-400">
                <Loader2 className="h-4 w-4 animate-spin" />
                Memuat jadwal…
              </CardContent>
            </Card>
          ) : !schedule ? (
            <Card>
              <CardContent className="py-10 text-center text-gray-400">
                Belum ada jadwal yang diaktifkan.
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between flex-wrap gap-2">
                  <div>
                    <CardTitle className="text-lg">{schedule.title}</CardTitle>
                    <CardDescription className="mt-1">
                      {schedule.meta.result.entriesGenerated} sesi &middot;{" "}
                      {schedule.meta.input.activeClasses} kelas &middot;{" "}
                      {schedule.meta.input.teachers} guru
                      {schedule.meta.result.violations > 0 && (
                        <span className="text-amber-600 ml-2">
                          &middot; {schedule.meta.result.violations} pelanggaran
                        </span>
                      )}
                    </CardDescription>
                  </div>
                  <Badge className="bg-green-100 text-green-800 border border-green-200 text-xs">
                    Aktif
                  </Badge>
                </div>
              </CardHeader>

              <CardContent>
                {/* Day selector */}
                <div className="flex flex-wrap gap-1.5 mb-5">
                  {DAYS.map((day) => (
                    <button
                      key={day}
                      onClick={() => setActiveDay(day)}
                      className={`px-3.5 py-1.5 rounded-md text-sm font-medium transition-colors ${
                        activeDay === day
                          ? "bg-blue-700 text-white shadow-sm"
                          : "bg-gray-100 text-gray-600 hover:bg-gray-200"
                      }`}
                    >
                      {DAY_LABELS[day]}
                    </button>
                  ))}
                </div>

                {timeSlots.length === 0 ? (
                  <p className="text-sm text-gray-400 py-6 text-center">
                    Tidak ada sesi pada hari {DAY_LABELS[activeDay]}.
                  </p>
                ) : (
                  <div className="overflow-x-auto rounded-lg border border-gray-100">
                    <table className="text-xs border-collapse w-full">
                      <thead>
                        <tr className="bg-gray-50">
                          <th className="px-3 py-2 border-b border-r font-semibold text-gray-500 whitespace-nowrap text-left sticky left-0 bg-gray-50 z-10">
                            Waktu
                          </th>
                          {classNames.map((cn) => (
                            <th
                              key={cn}
                              className="px-2 py-2 border-b font-semibold text-gray-700 text-center whitespace-nowrap min-w-[90px]"
                            >
                              {cn}
                            </th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {timeSlots.map((slot) => (
                          <tr key={slot} className="border-b last:border-0 hover:bg-blue-50/30 transition-colors">
                            <td className="px-3 py-2 border-r font-mono text-gray-500 whitespace-nowrap sticky left-0 bg-white z-10">
                              {slot}
                            </td>
                            {classNames.map((cn) => {
                              const entry = grid[slot]?.[cn];
                              return (
                                <td key={cn} className="px-2 py-2 text-center align-top">
                                  {entry ? (
                                    <div>
                                      <div className="font-medium text-gray-800 leading-tight">
                                        {entry.subjectName}
                                      </div>
                                      <div className="text-gray-400 text-[10px] mt-0.5 leading-tight">
                                        {entry.teacherName?.split(" ").slice(0, 2).join(" ")}
                                      </div>
                                    </div>
                                  ) : (
                                    <span className="text-gray-200">—</span>
                                  )}
                                </td>
                              );
                            })}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </section>

        {/* Algorithm Explanation */}
        <section>
          <h2 className="text-xl font-semibold mb-4 text-gray-800 flex items-center gap-2">
            <Cpu className="h-5 w-5 text-blue-600" />
            Tentang Sistem
          </h2>

          <div className="grid md:grid-cols-2 gap-5">

            <Card className="border-blue-100">
              <CardHeader className="pb-2">
                <CardTitle className="text-blue-700 flex items-center gap-2 text-base">
                  <Zap className="h-4 w-4" />
                  Genetic Algorithm (GA)
                </CardTitle>
              </CardHeader>
              <CardContent className="text-gray-600 text-sm leading-relaxed space-y-2">
                <p>
                  Genetic Algorithm meniru proses evolusi biologis untuk mencari solusi
                  penjadwalan yang optimal. Setiap jadwal direpresentasikan sebagai individu
                  dalam populasi.
                </p>
                <p>
                  Melalui proses <em>seleksi</em>, <em>crossover</em>, dan <em>mutasi</em>,
                  populasi jadwal berkembang dari generasi ke generasi menuju solusi dengan
                  konflik minimal.
                </p>
                <p>
                  GA unggul dalam mengeksplorasi ruang solusi yang sangat besar secara efisien.
                </p>
              </CardContent>
            </Card>

            <Card className="border-amber-100">
              <CardHeader className="pb-2">
                <CardTitle className="text-amber-700 flex items-center gap-2 text-base">
                  <Clock className="h-4 w-4" />
                  Tabu Search (TS)
                </CardTitle>
              </CardHeader>
              <CardContent className="text-gray-600 text-sm leading-relaxed space-y-2">
                <p>
                  Tabu Search adalah algoritma pencarian lokal yang menyempurnakan solusi
                  terbaik dari GA. TS bergerak antar solusi tetangga sambil menghindari
                  langkah yang baru saja dilakukan (<em>daftar tabu</em>).
                </p>
                <p>
                  Strategi ini memungkinkan TS keluar dari <em>optimum lokal</em> dan
                  menemukan solusi yang lebih baik daripada pencarian biasa.
                </p>
                <p>
                  TS melengkapi GA dengan menyempurnakan solusi secara lokal dan presisi.
                </p>
              </CardContent>
            </Card>

            <Card className="md:col-span-2 bg-blue-50 border-blue-200">
              <CardHeader className="pb-2">
                <CardTitle className="text-blue-800 text-base">Pendekatan Hibrid GA + TS</CardTitle>
              </CardHeader>
              <CardContent className="text-blue-900 text-sm leading-relaxed">
                <p>
                  Sistem ini menggabungkan kekuatan GA (eksplorasi global) dan TS (eksploitasi
                  lokal) dalam satu pendekatan terpadu. Pertama, GA menghasilkan jadwal yang
                  baik secara keseluruhan, lalu TS menyempurnakan hasilnya untuk mengurangi
                  konflik keras (dua guru mengajar bersamaan, kelas ganda di slot yang sama)
                  maupun pelanggaran lunak (preferensi jadwal tidak terpenuhi). Kombinasi ini
                  menghasilkan jadwal yang jauh lebih optimal dibandingkan menggunakan salah
                  satu algoritma saja.
                </p>
              </CardContent>
            </Card>

          </div>
        </section>

      </div>
    </div>
  );
}
