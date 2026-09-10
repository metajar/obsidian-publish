/**
 * Markdown embed parsing for image publishing.
 *
 * Recognises the two ways a note can embed an image:
 *   - Obsidian wiki embeds:   ![[image.png]]   ![[folder/image.png|400]]
 *     (the `|400` size suffix is dropped when rewriting; anything after the
 *     first `|` is treated as a display suffix and discarded)
 *   - standard local refs:    ![alt](relative/path.png)  (vault-relative)
 *
 * Remote `http(s)://` refs are left completely untouched, and embeds inside
 * code fences or inline code spans are ignored.
 *
 * This module is pure (no Obsidian imports) so it is trivially unit-testable;
 * vault resolution and uploading live in `imagePublisher.ts`.
 */

/** Extensions the server accepts for POST /api/assets. */
const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp"]);

/** A parsed embed that points at a local image file. */
export interface WikiImageEmbed {
  kind: "wiki";
  /** Raw target between `[[` and `|` — basename or vault path. */
  target: string;
  /** Alt text for the rewritten markdown (basename without extension). */
  alt: string;
  start: number;
  end: number;
}

/** A parsed `![alt](path)` ref that points at a local (non-remote) image. */
export interface RefImageEmbed {
  kind: "ref";
  /** Raw path between the parentheses — vault-relative. */
  target: string;
  alt: string;
  start: number;
  end: number;
}

export type MarkdownImageEmbed = WikiImageEmbed | RefImageEmbed;

export interface EmbedScan {
  /** Local image embeds, in document order. */
  images: MarkdownImageEmbed[];
  /** Names of embedded attachments that are not images (pdf, md, mp3, …). */
  nonImages: string[];
}

/** Whether `filename` has an extension the server accepts as an image. */
export function isImageFilename(filename: string): boolean {
  const dot = filename.lastIndexOf(".");
  if (dot < 0) return false;
  return IMAGE_EXTENSIONS.has(filename.slice(dot + 1).toLowerCase());
}

const WIKI_EMBED_RE = /!\[\[([^\[\]]+?)\]\]/g;
const REF_EMBED_RE = /!\[([^\]\n]*)\]\(([^)\n]+)\)/g;
const FENCED_CODE_RE = /(^|\n)([ \t]*(?:```|~~~)[^\n]*(?:\n|$)[\s\S]*?)([ \t]*(?:```|~~~)[ \t]*(?=\n|$))/g;
const INLINE_CODE_RE = /`[^`\n]*`/g;

/** Character ranges of the markdown that are code and must not be rewritten. */
function codeRanges(markdown: string): Array<[number, number]> {
  const ranges: Array<[number, number]> = [];
  for (const re of [FENCED_CODE_RE, INLINE_CODE_RE]) {
    re.lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = re.exec(markdown)) !== null) {
      ranges.push([match.index, match.index + match[0].length]);
    }
  }
  return ranges;
}

function inRanges(ranges: Array<[number, number]>, start: number, end: number): boolean {
  return ranges.some(([rs, re]) => start < re && end > rs);
}

/**
 * Scan markdown for image embeds and non-image attachments.
 * Remote (`http://` / `https://`) refs are ignored entirely.
 */
export function scanEmbeds(markdown: string): EmbedScan {
  const code = codeRanges(markdown);
  const scan: EmbedScan = { images: [], nonImages: [] };

  WIKI_EMBED_RE.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = WIKI_EMBED_RE.exec(markdown)) !== null) {
    const start = match.index;
    const end = start + match[0].length;
    if (inRanges(code, start, end)) continue;
    // Anything after the first `|` is a display suffix (`|400`, `|400x300`).
    const target = match[1].split("|", 1)[0].trim();
    if (!target || /^[a-z][a-z0-9+.-]*:\/\//i.test(target)) continue;
    if (isImageFilename(target)) {
      const basename = target.split("/").pop() ?? target;
      scan.images.push({
        kind: "wiki",
        target,
        alt: basename.replace(/\.[^.]+$/, ""),
        start,
        end,
      });
    } else {
      scan.nonImages.push(basenameOf(target));
    }
  }

  REF_EMBED_RE.lastIndex = 0;
  while ((match = REF_EMBED_RE.exec(markdown)) !== null) {
    const start = match.index;
    const end = start + match[0].length;
    if (inRanges(code, start, end)) continue;
    const alt = match[1];
    const target = match[2].trim();
    // Remote images are hosted elsewhere already — leave them untouched.
    if (/^https?:\/\//i.test(target)) continue;
    if (!target) continue;
    if (isImageFilename(target)) {
      scan.images.push({ kind: "ref", target, alt, start, end });
    } else {
      scan.nonImages.push(basenameOf(target));
    }
  }

  // Both regexes ran over the same spans: sort into document order so the
  // rewrite transform can consume them front-to-back.
  scan.images.sort((a, b) => a.start - b.start);
  return scan;
}

function basenameOf(path: string): string {
  return path.split("/").pop() ?? path;
}

/**
 * Rewrite every image embed for which `urlFor` returns a URL to
 * `![alt](<url>)`, leaving all other text (and unresolved embeds) untouched.
 * Works on the outbound payload only — the note on disk is never modified.
 * (Deliberate product decision: publishing must not mutate the vault.)
 *
 * `urlFor` returning null/undefined means "leave this embed as written".
 */
export function rewriteImageEmbeds(
  markdown: string,
  embeds: readonly MarkdownImageEmbed[],
  urlFor: (embed: MarkdownImageEmbed) => string | null | undefined,
): string {
  // Replace from the back so earlier offsets stay valid.
  const ordered = [...embeds].sort((a, b) => b.start - a.start);
  let out = markdown;
  for (const embed of ordered) {
    const url = urlFor(embed);
    if (!url) continue;
    const replacement = `![${embed.alt.replace(/[\[\]]/g, "")}](${url})`;
    out = out.slice(0, embed.start) + replacement + out.slice(embed.end);
  }
  return out;
}
