import { requestUrl } from "obsidian";

/**
 * Typed client for the self-hosted publish server.
 *
 * Implements the frozen API contract (PRD §10, minus theme):
 *   GET    /api/routes/{route}/available
 *   POST   /api/pages
 *   PUT    /api/pages/{route}
 *   GET    /api/pages
 *   DELETE /api/pages/{route}
 *
 * Every request carries `Authorization: Bearer <token>`. Every method returns
 * a discriminated `ApiResult` — errors are mapped to `ApiError` and never
 * thrown, so callers can switch on `error.kind` for user-facing feedback.
 *
 * SECURITY: never log the token, page passwords, or page content here.
 */

export interface PageRecord {
  route: string;
  title: string;
  created_at: string;
  updated_at: string;
  password_protected: boolean;
  /** Optional: if the server returns the absolute live URL, prefer it. */
  url?: string;
}

export interface PublishPageInput {
  title: string;
  markdown: string;
  /**
   * Password semantics (PUT only; on POST, omit when not protecting):
   * - string  → set / replace the page password
   * - null    → remove password protection (PUT only)
   * - undefined → keep the existing password unchanged (PUT only)
   */
  password?: string | null;
}

export type ApiError =
  | { kind: "bad-config"; message: string }
  | { kind: "unauthorized"; status: number }
  | { kind: "route-taken"; status: number; message?: string }
  | { kind: "invalid-route"; status: number; message?: string }
  | { kind: "not-found"; status: number }
  | { kind: "server-error"; status: number; message?: string }
  | { kind: "unreachable"; message: string }
  | { kind: "unknown"; message: string };

export type ApiResult<T> = { ok: true; data: T } | { ok: false; error: ApiError };

/** Human-readable message for an ApiError, suitable for a Notice. */
export function describeApiError(error: ApiError): string {
  switch (error.kind) {
    case "bad-config":
      return `Publish: ${error.message}`;
    case "unauthorized":
      return "Publish: invalid API token — check it in Publish plugin settings.";
    case "route-taken":
      return `Publish: that route is already taken. ${error.message ?? ""}`.trim();
    case "invalid-route":
      return `Publish: server rejected the route. ${error.message ?? ""}`.trim();
    case "not-found":
      return "Publish: page not found on the server — it may have been unpublished elsewhere.";
    case "server-error":
      return `Publish: server error (${error.status}). ${error.message ?? ""}`.trim();
    case "unreachable":
      return `Publish: could not reach the server — is it running, and is the URL correct? (${error.message})`;
    case "unknown":
      return `Publish: unexpected error — ${error.message}`;
  }
}

interface RequestOptions {
  method: "GET" | "POST" | "PUT" | "DELETE";
  path: string;
  body?: unknown;
}

interface RawResponse {
  status: number;
  text: string;
  json: Record<string, unknown> | null;
}

/**
 * Map a thrown error or non-2xx response to an ApiError.
 * Exported for unit testing.
 */
export function mapError(err: unknown): ApiError {
  // requestUrl rejects with an Error that carries `status` for HTTP failures
  // (and a plain Error for connection failures). We normalise both, plus a
  // synthetic {status} object used for resolved-but-failing responses.
  const status =
    typeof err === "object" && err !== null && "status" in err && typeof (err as { status: unknown }).status === "number"
      ? (err as { status: number }).status
      : null;

  if (status !== null) {
    const message =
      typeof err === "object" && err !== null && "message" in err && typeof (err as { message: unknown }).message === "string"
        ? (err as { message: string }).message
        : undefined;
    return mapStatus(status, message);
  }

  const message = err instanceof Error ? err.message : String(err);
  return { kind: "unreachable", message };
}

function mapStatus(status: number, message?: string): ApiError {
  switch (true) {
    case status === 401:
    case status === 403:
      return { kind: "unauthorized", status };
    case status === 409:
      return { kind: "route-taken", status, message };
    case status === 400:
      return { kind: "invalid-route", status, message };
    case status === 404:
      return { kind: "not-found", status };
    case status >= 500:
      return { kind: "server-error", status, message };
    default:
      return { kind: "server-error", status, message };
  }
}

export class PublishApiClient {
  constructor(
    private readonly baseUrl: string,
    private readonly token: string,
  ) {}

  /** Check with the server whether `route` is free. */
  async checkRouteAvailable(route: string): Promise<ApiResult<boolean>> {
    const res = await this.request({
      method: "GET",
      path: `/api/routes/${encodeURIComponent(route)}/available`,
    });
    if (!res.ok) return res;
    const available = res.data.json?.["available"];
    if (typeof available !== "boolean") {
      return {
        ok: false,
        error: { kind: "unknown", message: "server returned a malformed availability response" },
      };
    }
    return { ok: true, data: available };
  }

  /** Publish a new page. Password is omitted from the payload when not set. */
  async createPage(route: string, input: PublishPageInput): Promise<ApiResult<PageRecord>> {
    const payload: Record<string, unknown> = {
      route,
      title: input.title,
      markdown: input.markdown,
    };
    if (input.password !== undefined && input.password !== null) {
      payload["password"] = input.password;
    }
    return this.expectPage(
      await this.request({ method: "POST", path: "/api/pages", body: payload }),
    );
  }

  /**
   * Update an existing page at `route`.
   * `password` semantics: undefined = keep, string = set/replace, null = remove.
   */
  async updatePage(route: string, input: PublishPageInput): Promise<ApiResult<PageRecord>> {
    const payload: Record<string, unknown> = {
      title: input.title,
      markdown: input.markdown,
      password: input.password === undefined ? undefined : input.password,
    };
    // Only include `password` when it is a string (set/replace) or null (remove).
    if (payload["password"] === undefined) delete payload["password"];
    return this.expectPage(
      await this.request({ method: "PUT", path: `/api/pages/${encodeURIComponent(route)}`, body: payload }),
    );
  }

  /** List all published pages, with metadata. Live server truth. */
  async listPages(): Promise<ApiResult<PageRecord[]>> {
    const res = await this.request({ method: "GET", path: "/api/pages" });
    if (!res.ok) return res;
    const body = res.data.json ?? {};
    const list = body["pages"] ?? body["data"] ?? (Array.isArray(res.data.json) ? res.data.json : null);
    if (!Array.isArray(list)) {
      return {
        ok: false,
        error: { kind: "unknown", message: "server returned a malformed page list" },
      };
    }
    return { ok: true, data: list.filter((p): p is PageRecord => typeof p === "object" && p !== null) };
  }

  /** Unpublish the page at `route`. */
  async deletePage(route: string): Promise<ApiResult<null>> {
    const res = await this.request({
      method: "DELETE",
      path: `/api/pages/${encodeURIComponent(route)}`,
    });
    if (!res.ok) return res;
    return { ok: true, data: null };
  }

  private async expectPage(res: ApiResult<RawResponse>): Promise<ApiResult<PageRecord>> {
    if (!res.ok) return res;
    const body = res.data.json;
    // Some servers wrap the record: prefer a nested `page` if present.
    const record = (body?.["page"] as Record<string, unknown> | undefined) ?? body;
    if (typeof record !== "object" || record === null || typeof record["route"] !== "string") {
      return {
        ok: false,
        error: { kind: "unknown", message: "server returned a malformed page record" },
      };
    }
    return { ok: true, data: record as unknown as PageRecord };
  }

  /**
   * Core request wrapper: adds the bearer header, performs the call via
   * Obsidian's requestUrl (bypasses CORS in the desktop app), and maps any
   * failure to a typed ApiError.
   */
  private async request(opts: RequestOptions): Promise<ApiResult<RawResponse>> {
    const base = this.baseUrl.trim().replace(/\/+$/, "");
    if (!base || !this.token.trim()) {
      return {
        ok: false,
        error: {
          kind: "bad-config",
          message: "server URL and API token must be set in the Publish plugin settings first.",
        },
      };
    }
    const url = `${base}${opts.path}`;
    try {
      const res = await requestUrl({
        url,
        method: opts.method,
        headers: {
          "Authorization": `Bearer ${this.token.trim()}`,
          "Content-Type": "application/json",
        },
        body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
        throw: false,
      });
      // `throw: false` asks requestUrl not to reject on HTTP error statuses,
      // but older Obsidian builds may still reject; the catch below handles that.
      if (res.status < 200 || res.status >= 300) {
        return { ok: false, error: mapStatus(res.status, errorBodyMessage(res)) };
      }
      return { ok: true, data: { status: res.status, text: res.text, json: safeJson(res) } };
    } catch (err) {
      return { ok: false, error: mapError(err) };
    }
  }
}

function safeJson(res: { json?: unknown; text: string }): Record<string, unknown> | null {
  if (res.json !== undefined && res.json !== null && typeof res.json === "object" && !Array.isArray(res.json)) {
    return res.json as Record<string, unknown>;
  }
  if (Array.isArray(res.json)) return res.json as unknown as Record<string, unknown>;
  try {
    return JSON.parse(res.text) as Record<string, unknown>;
  } catch {
    return null;
  }
}

function errorBodyMessage(res: { json?: unknown; text: string }): string | undefined {
  const json = safeJson(res);
  const msg = json?.["error"] ?? json?.["message"];
  if (typeof msg === "string" && msg.length > 0) return msg;
  if (res.text) {
    const short = res.text.trim().slice(0, 200);
    if (short) return short;
  }
  return undefined;
}
