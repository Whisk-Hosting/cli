// Customer accounts from the storefront's side: signing in with a password, and the checks on
// a new account's details. A code sign-in is the API's own (src/api/auth/sign-in-code).
import type { api } from "./medusa"

type Api = ReturnType<typeof api>

export const MIN_PASSWORD = 10

export const passwordProblem = (password: string): string | null =>
  password.length < MIN_PASSWORD ? `Use at least ${MIN_PASSWORD} characters for the password.` : null

export const registerProblem = (input: { email: string; password: string; first_name: string; last_name: string }): string | null => {
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(input.email)) return "Check your email address."
  if (!input.first_name || !input.last_name) return "Fill in your name."
  return passwordProblem(input.password)
}

// signIn swaps an email and password for a session: Medusa answers a token, and the token makes
// the session cookie (collected in the call, for the caller to pass on).
export const signIn = async (a: Api, email: string, password: string) => {
  const { token } = await a.post<{ token: string }>("/auth/customer/emailpass", { email, password })
  await a.post("/auth/session", {}, token)
}
