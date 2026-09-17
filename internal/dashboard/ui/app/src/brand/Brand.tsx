import logoAnimated from "./logo-animated.webp";
import logoStill from "./logo-still.png";

const prefersReducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;

export const BRAND_LABEL = "Robot Dreams Mission Control — display options";

export const Brand = (
  <>
    <img
      className="mc-brand-mark"
      src={prefersReducedMotion ? logoStill : logoAnimated}
      alt=""
      width={30}
      height={30}
    />
    <span className="mc-brand-text">
      <span className="mc-brand-line1">Robot Dreams</span>
      <span className="mc-brand-line2">Mission Control</span>
    </span>
  </>
);
