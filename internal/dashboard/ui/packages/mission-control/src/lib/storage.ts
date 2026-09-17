let prefix = "dream.";

export function setStoragePrefix(p: string): void {
  prefix = p;
}

export function storageKey(key: string): string {
  return prefix + key;
}

export function readStored(key: string): string | null {
  try {
    return localStorage.getItem(prefix + key);
  } catch {
    return null;
  }
}

export function writeStored(key: string, value: string): void {
  try {
    localStorage.setItem(prefix + key, value);
  } catch {
    return;
  }
}

export function removeStored(key: string): void {
  try {
    localStorage.removeItem(prefix + key);
  } catch {
    return;
  }
}
