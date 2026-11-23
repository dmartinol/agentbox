// Terminal streams arrive with SGR color codes, cursor controls, private
// modes (synchronized output, kitty keyboard reports), window titles, and
// carriage returns. The drawer is a text view, so keep the content and drop
// every control sequence instead of showing escape fragments verbatim.
const OSC = /\u001B\][^\u0007\u001B]*(?:\u0007|\u001B\\)/g;
const STRING = /\u001B[P^_X][^\u001B]*(?:\u001B\\)?/g;
const CSI = /(?:\u001B\[|\u009B)[0-?]*[ -/]*[@-~]/g;
const ESCAPE = /\u001B[()*+#%][0-9A-Za-z@]|\u001B[=>78cDEHMNOZ\\]/g;

function applyCarriageReturns(line: string): string {
  const parts = line.split('\r');
  let output = '';
  for (const part of parts) {
    output = part.length >= output.length ? part : part + output.slice(part.length);
  }
  return output;
}

function applyBackspaces(line: string): string {
  const output: string[] = [];
  for (const character of line) {
    if (character === '\b') output.pop();
    else output.push(character);
  }
  return output.join('');
}

export function normalizeTerminalText(input: string | undefined): string {
  if (!input) return '';
  const text = input
    .replace(/\r\n/g, '\n')
    .replace(OSC, '')
    .replace(STRING, '')
    .replace(CSI, '')
    .replace(ESCAPE, '')
    .split('\n')
    .map((line) => applyBackspaces(applyCarriageReturns(line)))
    .join('\n');
  // Drop remaining control and C1 characters except tab and newline.
  return text.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F\u0080-\u009F]/g, '');
}

/**
 * Explains non-rendered log text. Rendered tmux captures need no note; the
 * fallbacks do, so a garbled raw stream is not mistaken for agent output.
 */
export function logSourceNote(source?: string): string {
  if (source === 'snapshot') return 'Last rendered screen; the agent session has ended.';
  if (source === 'stream') return 'Raw terminal stream; this run has no rendered screen, so the text may be garbled.';
  return '';
}

export function middleEllipsis(value: string, max = 44): string {
  if (value.length <= max) return value;
  const head = Math.ceil((max - 1) / 2);
  const tail = Math.floor((max - 1) / 2);
  return `${value.slice(0, head)}…${value.slice(-tail)}`;
}

export function formatSyncTimestamp(value?: string): string {
  if (!value) return 'Never';
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return 'Never';
  return date.toLocaleString();
}

/** The repository's short name, used in text badges and option labels. */
export function repoName(repo?: string): string {
  const parts = (repo ?? '').split('/').filter(Boolean);
  return parts[parts.length - 1] ?? 'repository';
}

function versionSegment(segment?: string): number {
  const match = segment ? /^\d+/.exec(segment) : null;
  return match ? Number(match[0]) : -1;
}

/** Numeric ordering for dot-separated release versions. */
export function compareVersions(left: string, right: string): number {
  const leftParts = left.split('.');
  const rightParts = right.split('.');
  const length = Math.max(leftParts.length, rightParts.length);
  for (let index = 0; index < length; index += 1) {
    const leftNumber = versionSegment(leftParts[index]);
    const rightNumber = versionSegment(rightParts[index]);
    if (leftNumber !== rightNumber) return leftNumber - rightNumber;
  }
  return left.localeCompare(right);
}
