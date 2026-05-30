"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { signOut, useSession } from "next-auth/react";
import { Button } from "@/components/ui/button";
import { LogOut } from "lucide-react";
import { cn } from "@/lib/utils";

const ROLE_LABEL: Record<string, string> = {
  admin: "Administrator",
  teacher: "Guru",
  student: "Siswa",
};

export function Nav() {
  const { data: session } = useSession();
  const pathname = usePathname();
  const isAdmin = session?.role === "admin";

  const links = isAdmin
    ? [
        { href: "/dashboard", label: "Dashboard" },
        { href: "/jadwal", label: "Jadwal Tersimpan" },
      ]
    : [{ href: "/jadwal", label: "Jadwal" }];

  return (
    <nav className="bg-blue-800 shadow-md">
      <div className="max-w-7xl mx-auto px-4 flex items-center justify-between h-14">
        {/* Left: logo + links */}
        <div className="flex items-center gap-5">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-full bg-white flex items-center justify-center overflow-hidden shrink-0 shadow-sm">
              <Image
                src="/logo.png"
                alt="Logo SMP Mater Dei"
                width={32}
                height={32}
                className="object-contain p-0.5"
              />
            </div>
            <span className="font-bold text-white text-sm hidden sm:block leading-tight">
              SMP Mater Dei
            </span>
          </div>

          <div className="flex gap-1">
            {links.map((l) => (
              <Link
                key={l.href}
                href={l.href}
                className={cn(
                  "px-3 py-1.5 rounded-md text-sm font-medium transition-colors",
                  pathname === l.href || pathname?.startsWith(l.href + "/")
                    ? "bg-amber-400 text-blue-900"
                    : "text-blue-200 hover:text-white hover:bg-blue-700"
                )}
              >
                {l.label}
              </Link>
            ))}
          </div>
        </div>

        {/* Right: role badge + logout */}
        <div className="flex items-center gap-3">
          <span className="text-xs px-2.5 py-1 rounded-full bg-blue-700 text-blue-100 font-medium hidden sm:block">
            {ROLE_LABEL[session?.role ?? ""] ?? session?.role}
          </span>
          <Button
            variant="ghost"
            size="sm"
            className="text-blue-200 hover:text-white hover:bg-blue-700"
            onClick={() => signOut({ callbackUrl: "/login" })}
          >
            <LogOut className="h-4 w-4 mr-1" />
            Keluar
          </Button>
        </div>
      </div>
    </nav>
  );
}
