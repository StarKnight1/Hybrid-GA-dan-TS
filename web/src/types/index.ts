export type UserRole = "admin" | "teacher" | "student";

export interface LoginResponse {
  token: string;
  role: UserRole;
}

export interface ScheduleEntry {
  teacherId?: number;
  subjectId: number;
  classId: number;
  subjectName: string;
  teacherName: string;
  className: string;
  day: string;
  timeStart: string;
  timeEnd: string;
}

export interface SoftBreakdown {
  sameDaySplit: number;
  sameDaySplitGrouped: number;
  pjokAfterDeadline: number;
}

export interface ResultStats {
  entriesGenerated: number;
  violations: number;
  unplaced: number;
  softBreakdown: SoftBreakdown;
}

export interface InputStats {
  activeAssignments: number;
  activeClasses: number;
  teachers: number;
}

export interface ScheduleMeta {
  input: InputStats;
  result: ResultStats;
  totalElapsedMs: number;
  loopCount?: number;
}

export interface GenerateResult {
  entries: ScheduleEntry[];
  meta: ScheduleMeta;
}

export interface SavedScheduleListItem {
  id: number;
  title: string;
  createdAt: string;
  createdBy: string;
  isActive: boolean;
}

export interface UserProfile {
  username: string;
  role: string;
  className?: string;   // student only
  classId?: number;     // student only
  teacherName?: string; // teacher only
  teacherId?: number;   // teacher only
}

export interface SavedSchedule {
  id: number;
  title: string;
  entries: ScheduleEntry[];
  meta: ScheduleMeta;
  createdAt: string;
  createdBy: string;
}

export interface DataStatus {
  teachers: number;
  activeClasses: number;
  subjects: number;
  teachingAssignments: number;
}

export type DayKey = "monday" | "tuesday" | "wednesday" | "thursday" | "friday";

export const DAY_LABELS: Record<DayKey, string> = {
  monday: "Senin",
  tuesday: "Selasa",
  wednesday: "Rabu",
  thursday: "Kamis",
  friday: "Jumat",
};

export const DAYS: DayKey[] = ["monday", "tuesday", "wednesday", "thursday", "friday"];
