export interface OperationIntent {
  classroomId: string;
  type: string;
  seatIds: string[];
}

interface IdempotencyKeyStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

interface OperationIdempotencyKeyManagerOptions {
  createKey?: () => string;
  storage?: IdempotencyKeyStorage;
  storageKey?: string;
}

export function createOperationIntentSignature(intent: OperationIntent): string {
  const seatIds = [...new Set(intent.seatIds)].sort();
  return JSON.stringify([intent.classroomId, intent.type, seatIds]);
}

export class OperationIdempotencyKeyManager {
  private active: { intent: string; key: string } | undefined;
  private readonly createKey: () => string;
  private readonly storage?: IdempotencyKeyStorage;
  private readonly storageKey?: string;

  constructor(options: OperationIdempotencyKeyManagerOptions = {}) {
    this.createKey = options.createKey ?? (() => crypto.randomUUID());
    this.storage = options.storage;
    this.storageKey = options.storageKey;
    this.active = this.readStoredState();
  }

  synchronize(intent: string): void {
    if (this.active && this.active.intent !== intent) {
      this.active = undefined;
      this.persist();
    }
  }

  keyFor(intent: string): string {
    this.synchronize(intent);
    if (!this.active) {
      this.active = { intent, key: this.createKey() };
      this.persist();
    }
    return this.active.key;
  }

  acknowledge(intent: string): void {
    if (this.active?.intent === intent) {
      this.active = undefined;
      this.persist();
    }
  }

  private readStoredState(): { intent: string; key: string } | undefined {
    if (!this.storage || !this.storageKey) return undefined;
    try {
      const raw = this.storage.getItem(this.storageKey);
      if (!raw) return undefined;
      const value: unknown = JSON.parse(raw);
      if (
        typeof value === 'object' &&
        value !== null &&
        'intent' in value &&
        'key' in value &&
        typeof value.intent === 'string' &&
        typeof value.key === 'string'
      ) {
        return { intent: value.intent, key: value.key };
      }
    } catch {
      // Storage is an optional durability enhancement; memory-only reuse remains safe.
    }
    return undefined;
  }

  private persist(): void {
    if (!this.storage || !this.storageKey) return;
    try {
      if (this.active) {
        this.storage.setItem(this.storageKey, JSON.stringify(this.active));
      } else {
        this.storage.removeItem(this.storageKey);
      }
    } catch {
      // Private browsing policies or quota failures must not block task submission.
    }
  }
}
