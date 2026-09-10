/**
 * Image publishing: resolve a note's image embeds against the vault, upload
 * each unique file to POST /api/assets, and rewrite the embeds to server URLs.
 *
 * PRODUCT DECISION: the note file on disk is NEVER modified. Rewriting happens
 * on the outbound markdown payload only — publishing is a transport concern
 * and must not mutate the user's vault. Re-publishing re-uploads (the server
 * dedupes and returns the existing URL) and rewrites again.
 *
 * Fail-soft policy: an upload failure never blocks the publish. The embed is
 * left as written, and the caller surfaces a warning Notice.
 */

import { PublishApiClient } from "./api";
import { MarkdownImageEmbed, rewriteImageEmbeds, scanEmbeds } from "./embeds";

/**
 * Structural slice of the Obsidian Vault API this module needs — keeps the
 * module unit-testable with a plain fake instead of the full Obsidian typings.
 */
export interface VaultLike {
  getFiles(): ReadonlyArray<{ path: string; name: string }>;
  readBinary(file: { path: string; name: string }): Promise<ArrayBuffer>;
}

export interface ImagePublishOptions {
  /** Raw note markdown as read from disk. */
  markdown: string;
  vault: VaultLike;
  client: PublishApiClient;
  /** Configured server base URL, for building absolute image URLs. */
  baseUrl: string;
}

export interface ImagePublishOutcome {
  /** Markdown to send to the server (embeds rewritten where uploaded). */
  markdown: string;
  /** Names of files successfully uploaded this run (deduped per file). */
  uploaded: string[];
  /** Names of files whose upload failed; their embeds are left unrewritten. */
  failed: string[];
  /** How many non-image attachments were embedded (skipped, not published). */
  nonImageCount: number;
}

const MIME_BY_EXTENSION: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
};

function mimeFor(filename: string): string {
  const dot = filename.lastIndexOf(".");
  const ext = dot < 0 ? "" : filename.slice(dot + 1).toLowerCase();
  return MIME_BY_EXTENSION[ext] ?? "application/octet-stream";
}

function normalizedVaultPath(path: string): string {
  return path.replace(/^\.\//, "").replace(/\\/g, "/");
}

/**
 * Resolve `![[target]]` per Obsidian semantics: with a path separator, an
 * exact vault path; otherwise any file matching by basename.
 */
function resolveWikiTarget(
  vault: VaultLike,
  target: string,
): { path: string; name: string } | null {
  if (target.includes("/")) {
    const wanted = normalizedVaultPath(target);
    return vault.getFiles().find((f) => normalizedVaultPath(f.path) === wanted) ?? null;
  }
  return vault.getFiles().find((f) => f.name === target) ?? null;
}

/** Resolve `![](path)` as a vault-relative path (decoded, "./" stripped). */
function resolveRefTarget(
  vault: VaultLike,
  target: string,
): { path: string; name: string } | null {
  let decoded = target;
  try {
    decoded = decodeURIComponent(target);
  } catch {
    // Malformed percent-encoding — fall back to the raw path.
  }
  const wanted = normalizedVaultPath(decoded);
  return vault.getFiles().find((f) => normalizedVaultPath(f.path) === wanted) ?? null;
}

export async function publishNoteImages(options: ImagePublishOptions): Promise<ImagePublishOutcome> {
  const { markdown, vault, client, baseUrl } = options;
  const scan = scanEmbeds(markdown);

  const outcome: ImagePublishOutcome = {
    markdown,
    uploaded: [],
    failed: [],
    nonImageCount: scan.nonImages.length,
  };
  if (scan.images.length === 0) return outcome;

  // Resolve every embed (memoised by raw target), then upload each distinct
  // vault file at most once — two embeds of the same image share one upload.
  // Dedupe is keyed by vault path, not object identity, so different syntaxes
  // pointing at one file produce exactly one upload.
  const fileByTarget = new Map<string, { path: string; name: string } | null>();
  const fileByPath = new Map<string, { path: string; name: string }>();
  const urlByPath = new Map<string, string>();

  for (const embed of scan.images) {
    if (fileByTarget.has(embed.target)) continue;
    const file =
      embed.kind === "wiki"
        ? resolveWikiTarget(vault, embed.target)
        : resolveRefTarget(vault, embed.target);
    fileByTarget.set(embed.target, file);
    if (file && !fileByPath.has(file.path)) fileByPath.set(file.path, file);
  }

  for (const file of fileByPath.values()) {
    if (!file) continue;
    try {
      const data = await vault.readBinary(file);
      const result = await client.uploadAsset({ filename: file.name, mime: mimeFor(file.name), data });
      if (result.ok) {
        const url = /^[a-z][a-z0-9+.-]*:\/\//i.test(result.data.url)
          ? result.data.url
          : `${baseUrl.trim().replace(/\/+$/, "")}${result.data.url}`;
        urlByPath.set(file.path, url);
        outcome.uploaded.push(file.name);
      } else {
        outcome.failed.push(file.name);
      }
    } catch {
      // Fail soft: a read or transport error skips this one image only.
      outcome.failed.push(file.name);
    }
  }

  outcome.markdown = rewriteImageEmbeds(markdown, scan.images, (embed: MarkdownImageEmbed) => {
    const file = fileByTarget.get(embed.target);
    if (!file) return null;
    return urlByPath.get(file.path) ?? null;
  });

  return outcome;
}
