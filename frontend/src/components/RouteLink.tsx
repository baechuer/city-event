import type { MouseEvent, ReactNode } from 'react';

interface RouteLinkProps {
  href: string;
  className?: string;
  onRouteClick(event: MouseEvent<HTMLAnchorElement>): void;
  children: ReactNode;
}

export function RouteLink({ href, className, onRouteClick, children }: RouteLinkProps) {
  return <a className={className} href={href} onClick={onRouteClick}>{children}</a>;
}
