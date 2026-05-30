"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { signOut, useSession } from "next-auth/react";
import { Button } from "@/components/ui/button";
import { LogOut } from "lucide-react";
import { cn } from "@/lib/utils";

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
    <nav className="border-b bg-white">
      <div className="max-w-7xl mx-auto px-4 flex items-center justify-between h-14">
        <div className="flex items-center gap-6">
          <span className="font-semibold text-sm">SMP Mater Dei</span>
          <div className="flex gap-1">
            {links.map((l) => (
              <Link
                key={l.href}
                href={l.href}
                className={cn(
                  "px-3 py-1.5 rounded-md text-sm font-medium transition-colors",
                  pathname === l.href
                    ? "bg-gray-100 text-gray-900"
                    : "text-gray-600 hover:text-gray-900 hover:bg-gray-50"
                )}
              >
                {l.label}
              </Link>
            ))}
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm text-gray-500 capitalize">{session?.role}</span>
          <Button
            variant="ghost"
            size="sm"
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
