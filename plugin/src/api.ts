import { requestUrl } from "obsidian";

/**
 * Typed client for the self-hosted publish server.
 *
 * Implements the frozen API contract (PRD §10):
 *   GET    /api/routes/{route}/available
 *   POST   /api/pages
 *   PUT    /api/pages/{route}
 *   GET    /api/pages
 *   DELETE /api/pages/{route}  → 200 + {"deleted": route} (not an empty 204:
 *                                 Obsidian's requestUrl throws a JSON EOF
 *                                 error on bodies it cannot parse)
 *   GET    /api/theme
 *   POST   /api/theme
 *   POST   /api/assets   (multipart; images only)
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

/** Site theme as stored on the server (frozen contract: GET /api/theme). */
export interface ThemeRecord {
  css: string;
  name: string;
  updated_at: string;
}

/** Result of a successful asset upload (frozen contract: POST /api/assets). */
export interface AssetUploadResult {
  url: string;
  filename: string;
}

/** Local file to upload via POST /api/assets. */
export interface AssetUploadInput {
  /** File name as sent to the server, e.g. "screenshot.png". */
  filename: string;
  mime: string;
  data: ArrayBuffer;
}

export type ApiError =
  | { kind: "bad-config"; message: string }
  | { kind: "unauthorized"; status: number }
  | { kind: "route-taken"; status: number; message?: string }
  | { kind: "invalid-route"; status: number; message?: string }
  | { kind: "not-found"; status: number }
  | { kind: "asset-rejected"; status: number; message?: string }
  | { kind: "asset-too-large"; status: number; message?: string }
  | { kind: "server-error"; status: number; message?: string }
  | { kind: "unreachable"; message: string }
  | { kind: "unknown"; message: string };

export type ApiResult<T> = { ok: true; data: T } | { ok: false; error: ApiError };

/**
 * Human-readable message for an ApiError, suitable for a Notice.
 * `context` labels the action that failed (default "Publish").
 */
export function describeApiError(error: ApiError, context = "Publish"): string {
  switch (error.kind) {
    case "bad-config":
      return `${context}: ${error.message}`;
    case "unauthorized":
      return `${context}: invalid API token — check it in Publish plugin settings.`;
    case "route-taken":
      return `${context}: that route is already taken. ${error.message ?? ""}`.trim();
    case "invalid-route":
      return `${context}: server rejected the route. ${error.message ?? ""}`.trim();
    case "not-found":
      return `${context}: page not found on the server — it may have been unpublished elsewhere.`;
    case "asset-rejected":
      return `${context}: the server rejected the file (images only — png/jpg/jpeg/gif/webp). ${error.message ?? ""}`.trim();
    case "asset-too-large":
      return `${context}: the file is too large (max 10 MB). ${error.message ?? ""}`.trim();
    case "server-error":
      return `${context}: server error (${error.status}). ${error.message ?? ""}`.trim();
    case "unreachable":
      return `${context}: could not reach the server — is it running, and is the URL correct? (${error.message})`;
    case "unknown":
      return `${context}: unexpected error — ${error.message}`;
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

  /**
   * Password-only update: PUT `/api/pages/{route}` with just `{password}`.
   * Markdown, title, and theme stay untouched — used by the settings-table
   * password action. A string sets/replaces the password; null removes it.
   */
  async setPagePassword(route: string, password: string | null): Promise<ApiResult<PageRecord>> {
    return this.expectPage(
      await this.request({
        method: "PUT",
        path: `/api/pages/${encodeURIComponent(route)}`,
        body: { password },
      }),
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

  // -- Theme -------------------------------------------------------------------

  /**
   * Fetch the current site theme. An unset theme is reported by the server as
   * `{"css": "", "name": "default", ...}` and returned as-is.
   */
  async getTheme(): Promise<ApiResult<ThemeRecord>> {
    const res = await this.request({ method: "GET", path: "/api/theme" });
    if (!res.ok) return res;
    return this.expectTheme(res, { css: "", name: "default", updated_at: "" });
  }

  /** Set the site theme. Arbitrary CSS is allowed (self-hosted, single user). */
  async setTheme(css: string, name?: string): Promise<ApiResult<ThemeRecord>> {
    const payload: Record<string, unknown> = { css };
    if (name !== undefined) payload["name"] = name;
    const res = await this.request({ method: "POST", path: "/api/theme", body: payload });
    if (!res.ok) return res;
    // Tolerate a body-less 2xx: echo back what we sent.
    return this.expectTheme(res, { css, name: name ?? "custom", updated_at: "" });
  }

  private expectTheme(res: ApiResult<RawResponse>, fallback: ThemeRecord): ApiResult<ThemeRecord> {
    if (!res.ok) return res;
    const body = res.data.json;
    if (body !== null && typeof body["css"] === "string" && typeof body["name"] === "string") {
      return {
        ok: true,
        data: {
          css: body["css"] as string,
          name: body["name"] as string,
          updated_at: typeof body["updated_at"] === "string" ? (body["updated_at"] as string) : "",
        },
      };
    }
    if (!res.data.text.trim()) return { ok: true, data: fallback };
    return {
      ok: false,
      error: { kind: "unknown", message: "server returned a malformed theme response" },
    };
  }

  // -- Assets ------------------------------------------------------------------

  /**
   * Upload an image via POST /api/assets (multipart form, field `file`).
   * 201 on first upload; 200 with the same body on a dedupe re-upload — both
   * are success. 400 (non-image/svg) and 413 (oversize) map to dedicated
   * error kinds so callers can fail soft with a precise message.
   */
  async uploadAsset(input: AssetUploadInput): Promise<ApiResult<AssetUploadResult>> {
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

    const body = buildMultipartBody(input);
    try {
      const res = await requestUrl({
        url: `${base}/api/assets`,
        method: "POST",
        headers: {
          "Authorization": `Bearer ${this.token.trim()}`,
          "Content-Type": `multipart/form-data; boundary=${MULTIPART_BOUNDARY}`,
        },
        body,
        throw: false,
      });
      if (res.status !== 200 && res.status !== 201) {
        return { ok: false, error: mapAssetStatus(res.status, errorBodyMessage(res)) };
      }
      const json = safeJson(res);
      const url = json?.["url"];
      const filename = json?.["filename"];
      if (typeof url !== "string" || typeof filename !== "string") {
        return {
          ok: false,
          error: { kind: "unknown", message: "server returned a malformed asset response" },
        };
      }
      return { ok: true, data: { url, filename } };
    } catch (err) {
      // requestUrl on some builds rejects with an Error carrying `status`;
      // run those through the asset-specific mapping too.
      const status =
        typeof err === "object" && err !== null && "status" in err && typeof (err as { status: unknown }).status === "number"
          ? (err as { status: number }).status
          : null;
      if (status !== null) {
        return {
          ok: false,
          error: mapAssetStatus(status, err instanceof Error ? err.message : undefined),
        };
      }
      return { ok: false, error: mapError(err) };
    }
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

// -- Multipart helpers (POST /api/assets) -----------------------------------

/** Fixed boundary; CSS-free and improbable inside any uploaded image bytes. */
const MULTIPART_BOUNDARY = "----obsidian-publish-7f3a9b";

/**
 * Build a `multipart/form-data` body with a single `file` field, as an
 * ArrayBuffer so binary image data passes through requestUrl byte-exact.
 * Exported for unit testing the wire shape.
 */
export function buildMultipartBody(input: AssetUploadInput): ArrayBuffer {
  const encoder = new TextEncoder();
  // Escape quotes/newlines per RFC 7578 §4.2 so hostile filenames can't
  // smuggle extra parts.
  const safeName = input.filename.replace(/["\r\n]/g, "_");
  const head = encoder.encode(
    `--${MULTIPART_BOUNDARY}\r\n` +
      `Content-Disposition: form-data; name="file"; filename="${safeName}"\r\n` +
      `Content-Type: ${input.mime}\r\n` +
      `\r\n`,
  );
  const tail = encoder.encode(`\r\n--${MULTIPART_BOUNDARY}--\r\n`);
  const data = new Uint8Array(input.data);

  const out = new Uint8Array(head.length + data.length + tail.length);
  out.set(head, 0);
  out.set(data, head.length);
  out.set(tail, head.length + data.length);
  return out.buffer;
}

/**
 * Status mapping specific to asset uploads: 400 is a rejected file (not an
 * invalid route) and 413 is oversize. Everything else defers to mapStatus.
 */
function mapAssetStatus(status: number, message?: string): ApiError {
  if (status === 400) return { kind: "asset-rejected", status, message };
  if (status === 413) return { kind: "asset-too-large", status, message };
  return mapStatus(status, message);
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
