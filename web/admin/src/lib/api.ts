/**
 * ADR 0011 L2 client-side token handling.
 *
 * All admin API requests must carry the bearer token configured server-side
 * (either HUAN_ADMIN_TOKEN env, or the auto-generated one printed to stderr
 * when loopback + no env). apiFetch reads the token from sessionStorage,
 * attaches the Authorization header, and prompts the user on 401.
 *
 * sessionStorage (not localStorage): token is scoped to the current tab
 * and clears when the tab closes — limiting XSS exposure window.
 */

const TOKEN_KEY = 'huan-admin-token'

export function getToken(): string | null {
  try {
    return sessionStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

export function setToken(token: string): void {
  try {
    sessionStorage.setItem(TOKEN_KEY, token)
  } catch {
    // sessionStorage unavailable (private mode, etc.) — fall back to in-memory
    inMemoryToken = token
  }
}

export function clearToken(): void {
  try {
    sessionStorage.removeItem(TOKEN_KEY)
  } catch {
    inMemoryToken = null
  }
}

// In-memory fallback when sessionStorage is unavailable.
let inMemoryToken: string | null = null

function readToken(): string | null {
  return getToken() ?? inMemoryToken
}

/**
 * Prompt the user for the admin token. Returns the entered token or null
 * if the user cancelled. Uses window.prompt for simplicity — a modal
 * would be nicer but adds ShadcnDialog wiring for a one-time flow.
 */
function promptForToken(): string | null {
  const msg =
    'Admin authentication required.\n\n' +
    'Enter the token printed in the huan serve stderr output\n' +
    '(search for "admin panel token"), or the value of your\n' +
    'HUAN_ADMIN_TOKEN environment variable.'
  const input = window.prompt(msg, '')
  return input && input.trim() ? input.trim() : null
}

/**
 * apiFetch is a fetch() wrapper that auto-attaches the admin token.
 * On 401 it clears any stored token, prompts the user once, and retries
 * the original request. Subsequent 401s (e.g., user entered wrong token)
 * are returned to the caller as-is — caller typically surfaces the error.
 */
export async function apiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  const withAuth = (override?: RequestInit): RequestInit => {
    const token = readToken()
    if (!token) return { ...init, ...override }
    const headers = new Headers(override?.headers ?? init?.headers ?? {})
    headers.set('Authorization', `Bearer ${token}`)
    return { ...init, ...override, headers }
  }

  let response = await fetch(input, withAuth())

  if (response.status !== 401) {
    return response
  }

  // 401 — token missing or wrong. Prompt and retry once.
  clearToken()
  const entered = promptForToken()
  if (!entered) {
    return response // user cancelled; return original 401
  }
  setToken(entered)
  return fetch(input, withAuth())
}
