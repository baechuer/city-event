import type { Tone } from '../state';

export function Notice({ message, tone }: { message: string; tone: Tone }) {
  return (
    <div className={`notice-toast tone-${tone}`} aria-live="polite">
      <span>{message}</span>
    </div>
  );
}
