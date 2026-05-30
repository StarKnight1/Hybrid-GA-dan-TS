"use client";

import { useState } from "react";
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
    <div className="flex min-h-screen items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-6">
          <h1 className="text-2xl font-bold text-gray-900">SMP Mater Dei</h1>
          <p className="text-sm text-gray-500 mt-1">Sistem Penjadwalan Pelajaran</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Masuk</CardTitle>
            <CardDescription>Masukkan username dan password Anda</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="identifier">Username / NIS</Label>
                <Input
                  id="identifier"
                  type="text"
                  placeholder="admin / NIS"
                  value={identifier}
                  onChange={(e) => setIdentifier(e.target.value)}
                  required
                  autoFocus
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  placeholder="••••••••"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </div>
              <Button type="submit" className="w-full" disabled={loading}>
                {loading ? (
                  <><Loader2 className="mr-2 h-4 w-4 animate-spin" /> Masuk...</>
                ) : (
                  "Masuk"
                )}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
