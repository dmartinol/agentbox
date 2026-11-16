export const defaultApproval = 'unattended';

export const approvalOptions = [
  { value: 'prompted', label: 'prompted' },
  { value: 'unattended', label: 'unattended — no permission prompts' }
];

export function approvalLabel(value: string | undefined): string {
  if (!value) return '';
  return approvalOptions.find((option) => option.value === value)?.label ?? value;
}

export function approvalForProfile(profile: { approval?: string } | undefined): string {
  return profile?.approval || defaultApproval;
}
