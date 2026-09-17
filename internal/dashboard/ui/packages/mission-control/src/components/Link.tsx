import type { AnchorHTMLAttributes, MouseEvent } from "react";
import { useRouter } from "../state/RouterContext";
import type { Route } from "../lib/routes";

export interface LinkProps extends Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href"> {
  to: Route;
  replace?: boolean;
}

export function Link({ to, replace, onClick, target, children, ...rest }: LinkProps) {
  const { href, navigate } = useRouter();
  return (
    <a
      {...rest}
      target={target}
      href={href(to)}
      onClick={(e: MouseEvent<HTMLAnchorElement>) => {
        onClick?.(e);
        if (e.defaultPrevented) return;
        if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
        if (target && target !== "_self") return;
        e.preventDefault();
        navigate(to, { replace });
      }}
    >
      {children}
    </a>
  );
}
