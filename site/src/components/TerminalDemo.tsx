import { useEffect, useState } from "react";

const CONNECT_CMD = "ssh -p 2222 jobs.sharadregoti.com";

const FRAMES: { title: string; body: string[] }[] = [
  {
    title: "jobs.sharadregoti.com",
    body: [
      "Connecting to jobs.sharadregoti.com...",
      "Welcome to sshire — SSH Careers Demo",
    ],
  },
  {
    title: "Open Roles",
    body: [
      "> TUI Engineer (Demo)            Remote",
      "  SRE, On-Call Rotation (Demo)   Remote",
      "  Support Engineer (Demo)        Remote",
      "",
      "↑/↓ move   enter view   q quit",
    ],
  },
  {
    title: "TUI Engineer (Demo)",
    body: [
      "Remote · Full-time",
      "",
      "This is a demo posting for sshire, an",
      "open-source tool for applying to jobs",
      "over SSH instead of a web form.",
      "",
      "a apply   esc back   q quit",
    ],
  },
  {
    title: "Apply: TUI Engineer (Demo)",
    body: [
      "Name       Ada Lovelace",
      "Email      ada@example.com",
      "Why you?   I like terminals.",
      "",
      "enter submit   esc cancel",
    ],
  },
  {
    title: "sshire",
    body: [
      "",
      "  ✓ Application submitted. Thanks, Ada!",
      "",
      "  Connection closed.",
    ],
  },
];

const TYPE_SPEED_MS = 55;
const FRAME_HOLD_MS = 2200;
const RESTART_PAUSE_MS = 1600;

export default function TerminalDemo() {
  const [typed, setTyped] = useState("");
  const [phase, setPhase] = useState<"typing" | "connected">("typing");
  const [frameIndex, setFrameIndex] = useState(0);

  useEffect(() => {
    if (phase !== "typing") return;
    if (typed.length >= CONNECT_CMD.length) {
      const t = setTimeout(() => setPhase("connected"), 500);
      return () => clearTimeout(t);
    }
    const t = setTimeout(() => setTyped(CONNECT_CMD.slice(0, typed.length + 1)), TYPE_SPEED_MS);
    return () => clearTimeout(t);
  }, [typed, phase]);

  useEffect(() => {
    if (phase !== "connected") return;
    const isLast = frameIndex === FRAMES.length - 1;
    const delay = isLast ? RESTART_PAUSE_MS : FRAME_HOLD_MS;
    const t = setTimeout(() => {
      if (isLast) {
        setPhase("typing");
        setTyped("");
        setFrameIndex(0);
      } else {
        setFrameIndex((i) => i + 1);
      }
    }, delay);
    return () => clearTimeout(t);
  }, [phase, frameIndex]);

  const frame = FRAMES[frameIndex];

  return (
    <div className="w-full max-w-xl rounded-lg border border-[var(--rule)] bg-[var(--card)] shadow-[0_0_40px_rgba(61,220,106,0.08)] overflow-hidden">
      <div className="flex items-center gap-1.5 border-b border-[var(--rule)] px-3 py-2">
        <span className="h-2.5 w-2.5 rounded-full bg-[#ff5f57]" />
        <span className="h-2.5 w-2.5 rounded-full bg-[#febc2e]" />
        <span className="h-2.5 w-2.5 rounded-full bg-[#28c840]" />
        <span className="ml-2 font-mono text-xs text-[var(--dim)]">
          {phase === "typing" ? "ssh" : frame.title}
        </span>
      </div>
      <div className="min-h-[220px] px-4 py-4 font-mono text-[13px] leading-relaxed sm:text-sm">
        {phase === "typing" ? (
          <p className="text-[var(--fg)]">
            <span className="text-[var(--dim)]">$ </span>
            {typed}
            <span className="ml-0.5 inline-block h-4 w-2 translate-y-0.5 animate-pulse bg-[var(--accent)]" />
          </p>
        ) : (
          <div className="space-y-0.5">
            {frame.body.map((line, i) => (
              <p key={i} className={line.startsWith(">") ? "text-[var(--accent)]" : "text-[var(--fg)]"}>
                {line || " "}
              </p>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
