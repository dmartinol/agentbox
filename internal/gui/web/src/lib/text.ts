// Terminal streams arrive with SGR color codes, cursor controls, and carriage
// returns. The drawer is a text view, so keep the content and drop the control
// bytes instead of rendering escape sequences verbatim.
const OSC = /\u001B\][^\u0007\u001B]*(?:\u0007|\u001B\\)/g;
const CSI = /[\u001B\u009B][[\]()#;?]*(?:(?:(?:[a-zA-Z\d]*(?:;[-a-zA-Z\d/#&.:=?%@~_]+)*)?\u0007)|(?:(?:\d{1,4}(?:[;:]\d{0,4})*)?[\dA-PR-TZcf-nq-uy=><~]))/g;
const CHARSET = /\u001B[()][A-Za-z0-9]/g;

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
    .replace(CHARSET, '')
    .replace(CSI, '')
    .split('\n')
    .map((line) => applyBackspaces(applyCarriageReturns(line)))
    .join('\n');
  // Drop remaining control characters except tab and newline.
  return text.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, '');
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
