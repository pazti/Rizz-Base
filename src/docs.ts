import { marked } from "marked";
import overview from "../docs/overview.md?raw";
import installation from "../docs/installation.md?raw";
import cli from "../docs/cli.md?raw";
import nodeSdk from "../docs/node-sdk.md?raw";
import pythonSdk from "../python/README.md?raw";
import sdks from "../docs/sdks.md?raw";
import security from "../docs/security.md?raw";

export interface DocEntry {
  slug: string;
  title: string;
  kicker: string;
  blurb: string;
  badge: string;
  markdown: string;
}

export const docs: DocEntry[] = [
  {
    slug: "overview",
    title: "What is Rizzbase?",
    kicker: "START HERE",
    blurb: "Local-first model, vocabulary, data flow.",
    badge: "CORE",
    markdown: overview,
  },
  {
    slug: "installation",
    title: "Install the binary",
    kicker: "MANUAL SETUP",
    blurb: "Installers, verified binaries, and SDK engine discovery.",
    badge: "LINUX · MAC · WIN",
    markdown: installation,
  },
  {
    slug: "cli",
    title: "CLI + engine reference",
    kicker: "SOURCE OF TRUTH",
    blurb: "Every operation, request shape, errors.",
    badge: "12 OPS",
    markdown: cli,
  },
  {
    slug: "node-sdk",
    title: "Node.js SDK",
    kicker: "STABLE V1",
    blurb: "Install, quickstart, full API.",
    badge: "NPM",
    markdown: nodeSdk,
  },
  {
    slug: "python-sdk",
    title: "Python SDK",
    kicker: "STABLE V1",
    blurb: "Install Python, set up the engine, and run a quickstart.",
    badge: "PYTHON",
    markdown: pythonSdk,
  },
  {
    slug: "sdks",
    title: "SDKs in other languages",
    kicker: "PYTHON + BEYOND",
    blurb: "Supported SDKs and the shared engine protocol.",
    badge: "NODE · PYTHON",
    markdown: sdks,
  },
  {
    slug: "security",
    title: "Security and limits",
    kicker: "READ ONCE",
    blurb: "Passphrase, encryption, locks, V1 bounds.",
    badge: "MUST READ",
    markdown: security,
  },
];

marked.setOptions({ gfm: true, breaks: false });

export function renderDoc(markdown: string): string {
  return marked.parse(markdown, { async: false }) as string;
}
