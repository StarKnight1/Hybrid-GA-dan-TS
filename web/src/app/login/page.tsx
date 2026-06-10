"use client";

import { useState } from "react";
import Image from "next/image";
import { signIn } from "next-auth/react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Loader2 } from "lucide-react";

export default function LoginPage() {
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const router = useRouter();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      const result = await signIn("credentials", {
        identifier,
        password,
        redirect: false,
      });

      if (result?.error) {
        toast.error("Login gagal. Periksa username dan password.");
      } else {
        router.replace("/");
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="relative min-h-screen flex items-center justify-center overflow-hidden bg-blue-900 px-4 py-8">
      {/* Background decorative blobs */}
      <div className="absolute inset-0 pointer-events-none overflow-hidden">
        <div className="absolute -top-32 -right-32 w-96 h-96 rounded-full bg-blue-700/50 blur-3xl" />
        <div className="absolute -bottom-32 -left-32 w-96 h-96 rounded-full bg-blue-800/70 blur-3xl" />
        <div className="absolute top-1/2 left-1/4 w-64 h-64 rounded-full bg-amber-400/10 blur-2xl" />
      </div>

      {/* Cross/pattern texture overlay */}
      <div
        className="absolute inset-0 opacity-5 pointer-events-none"
        style={{
          backgroundImage: `repeating-linear-gradient(
            45deg,
            #ffffff 0,
            #ffffff 1px,
            transparent 0,
            transparent 50%
          )`,
          backgroundSize: "24px 24px",
        }}
      />

      {/* Main content */}
      <div className="relative z-10 w-full max-w-sm flex flex-col items-center gap-6">

        {/* Logo + school identity */}
        <div className="text-center space-y-3">
          <div className="flex justify-center">
            <div className="w-28 h-28 rounded-full bg-white shadow-2xl flex items-center justify-center overflow-hidden ring-4 ring-amber-400/60">
              <Image
                src="/logo.png"
                alt="Logo SMP Mater Dei"
                width={108}
                height={108}
                className="object-contain p-1"
                priority
              />
            </div>
          </div>
          <div>
            <h1 className="text-2xl font-bold text-white tracking-wide leading-tight">
              Sistem Penjadwalan Pelajaran
            </h1>
            <p className="text-blue-200 text-sm mt-1">SMP Mater Dei</p>
            <p className="text-amber-300 text-xs mt-1 italic font-medium tracking-widest uppercase">
              Tota Christi per Mariam
            </p>
          </div>
        </div>

        {/* Divider */}
        <div className="flex items-center gap-3 w-full">
          <div className="flex-1 h-px bg-blue-700" />
          <div className="w-1.5 h-1.5 rounded-full bg-amber-400" />
          <div className="flex-1 h-px bg-blue-700" />
        </div>

        {/* Login card */}
        <Card className="w-full shadow-2xl border-0 bg-white/95 backdrop-blur-sm">
          <CardHeader className="pb-3">
            <CardTitle className="text-blue-800 text-lg">Masuk</CardTitle>
            <CardDescription>Masukkan username dan password Anda</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="identifier" className="text-gray-700">
                  Username / NIS
                </Label>
                <Input
                  id="identifier"
                  type="text"
                  placeholder="admin / NIS"
                  value={identifier}
                  onChange={(e) => setIdentifier(e.target.value)}
                  required
                  autoFocus
                  className="border-blue-200 focus-visible:ring-blue-400"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="password" className="text-gray-700">
                  Password
                </Label>
                <Input
                  id="password"
                  type="password"
                  placeholder="••••••••"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  className="border-blue-200 focus-visible:ring-blue-400"
                />
              </div>
              <Button
                type="submit"
                className="w-full bg-blue-700 hover:bg-blue-800 text-white font-semibold mt-2"
                disabled={loading}
              >
                {loading ? (
                  <><Loader2 className="mr-2 h-4 w-4 animate-spin" />Masuk...</>
                ) : (
                  "Masuk"
                )}
              </Button>
            </form>
          </CardContent>
        </Card>

        <p className="text-blue-400/60 text-xs text-center">
          © {new Date().getFullYear()} SMP Mater Dei — Jadwal Pelajaran
        </p>
      </div>
    </div>
  );
}
