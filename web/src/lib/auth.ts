import { type NextAuthOptions } from "next-auth";
import CredentialsProvider from "next-auth/providers/credentials";
import { apiLogin } from "./api";

export const authOptions: NextAuthOptions = {
  providers: [
    CredentialsProvider({
      name: "Credentials",
      credentials: {
        identifier: { label: "Username / NIS", type: "text" },
        password: { label: "Password", type: "password" },
      },
      async authorize(credentials) {
        if (!credentials?.identifier || !credentials?.password) return null;
        try {
          const data = await apiLogin(credentials.identifier, credentials.password);
          return {
            id: credentials.identifier,
            name: credentials.identifier,
            token: data.token,
            role: data.role,
          };
        } catch {
          return null;
        }
      },
    }),
  ],
  callbacks: {
    async jwt({ token, user }) {
      if (user) {
        token.accessToken = (user as unknown as { token: string }).token;
        token.role = (user as unknown as { role: string }).role;
      }
      return token;
    },
    async session({ session, token }) {
      session.accessToken = token.accessToken as string;
      session.role = token.role as string;
      return session;
    },
  },
  pages: {
    signIn: "/login",
  },
  session: {
    strategy: "jwt",
  },
};
