/**
 * Minimal mock of the `obsidian` module for unit tests.
 *
 * Only what `src/api.ts` needs is implemented. `requestUrl` dispatches to a
 * handler installed per-test via `__setRequestUrlHandler`, so tests can make
 * it resolve with an HTTP status or reject with a network error.
 */

export type MockRequest = {
  url: string;
  method: string;
  headers?: Record<string, string>;
  /** JSON payloads arrive as strings; multipart asset uploads as ArrayBuffer. */
  body?: string | ArrayBuffer;
  throw?: boolean;
};

export type MockResponse = {
  status: number;
  text?: string;
  json?: unknown;
};

export type MockHandler = (req: MockRequest) => Promise<MockResponse>;

let handler: MockHandler = async () => {
  throw new Error("no requestUrl handler installed for this test");
};

export function __setRequestUrlHandler(h: MockHandler): void {
  handler = h;
}

export async function requestUrl(req: MockRequest): Promise<MockResponse & { text: string; json?: unknown }> {
  const res = await handler(req);
  const text = res.text ?? (res.json !== undefined ? JSON.stringify(res.json) : "");
  return { ...res, text };
}

export class Notice {
  constructor(_message: string, _timeout?: number) {}
}
