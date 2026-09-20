import { useEffect, useMemo, useState } from "react";

// Palettes mirror the registry in internal/tui/style.go. Keep them in sync —
// this is the page's whole claim about what the themes look like.
const THEMES = {
  black: { bg: "#000000", fg: "#e8e8e8", dim: "#6b6b6b", rule: "#3d3d3d", thumb: "#8a8a8a", dots: ["#ff5f57", "#febc2e", "#28c840"] },
  phosphor: { bg: "#020a04", fg: "#3bf07a", dim: "#1c7d41", rule: "#17532c", thumb: "#2bbd60", dots: ["#114a26", "#1c7d41", "#3bf07a"] },
  amber: { bg: "#0f0a02", fg: "#ffb000", dim: "#8a5f10", rule: "#4d3505", thumb: "#cc8c00", dots: ["#4d3505", "#a3700a", "#ffb000"] },
  paper: { bg: "#f5f2ea", fg: "#1b1a17", dim: "#6d6659", rule: "#cbc4b4", thumb: "#8d8676", dots: ["#d1443c", "#d99a1a", "#3f8f43"] },
  blueprint: { bg: "#06151f", fg: "#d3e8f7", dim: "#5a87a5", rule: "#1b3b52", thumb: "#4691bd", dots: ["#e0736c", "#d9a84e", "#57b8a9"] },
} as const;

type ThemeName = keyof typeof THEMES;
const THEME_NAMES = Object.keys(THEMES) as ThemeName[];

type Tone = "fg" | "dim";
type Line = { text: string; tone?: Tone; bold?: boolean; bar?: boolean };

// A scrollbar spanning content rows [from, to], with the thumb over
// [thumbFrom, thumbTo] — the same geometry internal/tui/style.go computes.
type Scrollbar = { from: number; to: number; thumbFrom: number; thumbTo: number };

type Frame = { label: string; lines: Line[]; scrollbar?: Scrollbar };

// Transcribed from `tmux capture-pane` against a running sshire, so the
// wrapping, spacing and key hints are the real ones rather than an
// approximation that drifts every time the TUI changes.
const FRAMES: Frame[] = [
  {
    label: "Open roles",
    lines: [
      { text: "Open roles", bold: true },
      { text: "" },
      { text: "› TUI Engineer (Demo)", bold: true },
      { text: "  SRE, On-Call Rotation (Demo)", tone: "dim" },
      { text: "  Support Engineer (Demo)", tone: "dim" },
      { text: "" },
      { text: "↑/↓ move  ·  enter open  ·  q/esc quit", tone: "dim" },
    ],
  },
  {
    label: "Job description",
    scrollbar: { from: 3, to: 12, thumbFrom: 3, thumbTo: 9 },
    lines: [
      { text: "TUI Engineer (Demo)", bold: true },
      { text: "Remote · Full-time", tone: "dim" },
      { text: "" },
      { text: "This is a demo posting for sshire, an open-source tool" },
      { text: "for applying to jobs over SSH instead of a web form." },
      { text: "Press \"a\" to try the apply form. Nothing gets emailed" },
      { text: "anywhere; it just lands in a local dashboard." },
      { text: "" },
      { text: "Every job you see here comes from a dozen lines of YAML." },
      { text: "There is no database, no admin panel, and no web form" },
      { text: "for a scraper to find." },
      { text: "" },
      { text: "Connecting without a real terminal gets rejected" },
      { text: "" },
      { text: "a apply  ·  ↑/↓ scroll  ·  esc roles  ·  q quit", tone: "dim" },
    ],
  },
  {
    label: "Scrolled",
    scrollbar: { from: 3, to: 12, thumbFrom: 6, thumbTo: 12 },
    lines: [
      { text: "TUI Engineer (Demo)", bold: true },
      { text: "Remote · Full-time", tone: "dim" },
      { text: "" },
      { text: "" },
      { text: "Every job you see here comes from a dozen lines of YAML." },
      { text: "There is no database, no admin panel, and no web form" },
      { text: "for a scraper to find." },
      { text: "" },
      { text: "Connecting without a real terminal gets rejected" },
      { text: "outright, which is enough to turn away the overwhelming" },
      { text: "majority of automated, spray-and-pray applications." },
      { text: "" },
      { text: "Real project: https://github.com/sharadregoti/sshire" },
      { text: "" },
      { text: "a apply  ·  ↑/↓ scroll  ·  esc roles  ·  q quit", tone: "dim" },
    ],
  },
  {
    label: "Apply form",
    lines: [
      { text: "  Name", tone: "dim" },
      { text: "  > Ada Lovelace" },
      { text: "" },
      { text: "  Email", tone: "dim" },
      { text: "  > ada@example.com" },
      { text: "" },
      { text: " Resume link (or GitHub/LinkedIn)", bar: true, bold: true },
      { text: " >", bar: true },
      { text: "" },
      { text: "  Anything else?", tone: "dim" },
      { text: "" },
      { text: "shift+tab back • enter next", tone: "dim" },
    ],
  },
  {
    label: "Submitted",
    lines: [
      { text: "Application submitted ✓", bold: true },
      { text: "" },
      { text: "Ada Lovelace applied to TUI Engineer (Demo). We'll be in" },
      { text: "touch at ada@example.com." },
      { text: "" },
      { text: "press any key to exit", tone: "dim" },
    ],
  },
];

const FRAME_HOLD_MS = 3600;

function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(query.matches);
    const onChange = (e: MediaQueryListEvent) => setReduced(e.matches);
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, []);
  return reduced;
}

export default function TerminalDemo() {
  const [theme, setTheme] = useState<ThemeName>("black");
  const [index, setIndex] = useState(0);
  const reducedMotion = usePrefersReducedMotion();

  useEffect(() => {
    if (reducedMotion) return;
    const t = setTimeout(() => setIndex((i) => (i + 1) % FRAMES.length), FRAME_HOLD_MS);
    return () => clearTimeout(t);
  }, [index, reducedMotion]);

  const palette = THEMES[theme];
  const frame = FRAMES[index];

  const rows = useMemo(
    () =>
      frame.lines.map((line, i) => {
        const bar = frame.scrollbar;
        let scroll: "thumb" | "track" | null = null;
        if (bar && i >= bar.from && i <= bar.to) {
          scroll = i >= bar.thumbFrom && i <= bar.thumbTo ? "thumb" : "track";
        }
        return { line, scroll };
      }),
    [frame],
  );

  return (
    <div className="flex w-full flex-col items-center gap-5">
      <div
        className="w-full overflow-x-auto"
        style={{ colorScheme: theme === "paper" ? "light" : "dark" }}
      >
        <div
          className="mx-auto w-fit border px-4 py-3 font-mono leading-relaxed"
          style={{
            background: palette.bg,
            borderColor: palette.rule,
            color: palette.fg,
            fontSize: "clamp(10px, 2.6vw, 13px)",
          }}
        >
          {/* window chrome: traffic lights + right-aligned title, then the rule */}
          <div className="flex items-center justify-between gap-8">
            <span className="flex gap-1.5" aria-hidden="true">
              {palette.dots.map((dot) => (
                <span key={dot} className="inline-block h-2 w-2 rounded-full" style={{ background: dot }} />
              ))}
            </span>
            <span style={{ color: palette.dim }}>sshire — Jobs</span>
          </div>
          <div className="mt-1.5 border-t" style={{ borderColor: palette.rule }} />

          <div className="mt-3 min-h-[15em]">
            {rows.map(({ line, scroll }, i) => (
              <div key={i} className="flex justify-between gap-3" style={{ whiteSpace: "pre" }}>
                <span
                  style={{
                    color: line.tone === "dim" ? palette.dim : palette.fg,
                    fontWeight: line.bold ? 600 : 400,
                  }}
                >
                  {line.bar && <span style={{ color: palette.rule }}>┃</span>}
                  {line.text || " "}
                </span>
                {scroll && (
                  <span aria-hidden="true" style={{ color: scroll === "thumb" ? palette.thumb : palette.rule }}>
                    │
                  </span>
                )}
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-center gap-x-5 gap-y-3">
        <div className="flex items-center gap-2" role="group" aria-label="Terminal theme">
          {THEME_NAMES.map((name) => (
            <button
              key={name}
              type="button"
              onClick={() => setTheme(name)}
              aria-pressed={theme === name}
              title={name}
              className="rounded-sm px-2 py-1 font-mono text-xs transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
              style={{
                color: theme === name ? "var(--fg)" : "var(--dim)",
                background: theme === name ? "var(--card)" : "transparent",
                outlineColor: "var(--accent)",
                border: `1px solid ${theme === name ? THEMES[name].thumb : "transparent"}`,
              }}
            >
              <span
                className="mr-1.5 inline-block h-2 w-2 translate-y-px rounded-full"
                style={{ background: THEMES[name].fg, boxShadow: `0 0 0 1px ${THEMES[name].rule}` }}
              />
              {name}
            </button>
          ))}
        </div>

        <div className="flex items-center gap-1.5" role="group" aria-label="Demo screen">
          {FRAMES.map((f, i) => (
            <button
              key={f.label}
              type="button"
              onClick={() => setIndex(i)}
              aria-label={f.label}
              aria-pressed={index === i}
              className="h-1.5 w-5 rounded-full transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
              style={{
                background: index === i ? "var(--accent)" : "var(--rule)",
                outlineColor: "var(--accent)",
              }}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
