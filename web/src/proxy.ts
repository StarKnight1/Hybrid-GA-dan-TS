import { withAuth } from "next-auth/middleware";
import { NextResponse } from "next/server";

export default withAuth(
  function proxy(req) {
    const role = req.nextauth.token?.role as string | undefined;
    const path = req.nextUrl.pathname;

    if (path.startsWith("/dashboard") && role !== "admin") {
      return NextResponse.redirect(new URL("/jadwal", req.url));
    }
    return NextResponse.next();
  },
  {
    callbacks: {
      authorized: ({ token }) => !!token,
    },
  }
);

export const config = {
  matcher: ["/dashboard/:path*", "/jadwal/:path*"],
};
