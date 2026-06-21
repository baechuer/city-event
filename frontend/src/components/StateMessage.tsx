import type { ReactNode } from 'react';

export function StateMessage({ title, detail, children }: { title: string; detail: string; children?: ReactNode }) {
  return (
    <div className="state-message">
      <strong>{title}</strong>
      <span>{detail}</span>
      {children}
    </div>
  );
}
