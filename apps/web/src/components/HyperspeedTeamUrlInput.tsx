import {
  GIFTED_SUBDOMAIN_APEX,
  GIFTED_TEAM_WWW_PREFIX,
  sanitizeTeamSubdomainInput,
} from "../constants/giftedDomain";

type Props = {
  id?: string;
  value: string;
  onChange: (subdomain: string) => void;
  disabled?: boolean;
  className?: string;
};

/**
 * Fixed https://www. … .hyperspeedapp.com with a single editable team label.
 */
export function HyperspeedTeamUrlInput({
  id,
  value,
  onChange,
  disabled,
  className,
}: Props) {
  // Keep the editable field only as wide as the label so `.hyperspeedapp.com` sits flush to the right of what you type (not pushed to the far edge of the row).
  const minChars = 8; // "yourteam"
  const inputSize = Math.max(minChars, value.length);

  return (
    <div
      className={[
        "flex min-h-10 w-full max-w-xl items-center rounded-sm border border-input bg-background px-2 font-mono text-sm outline-none ring-ring ring-offset-2 ring-offset-background focus-within:ring-2",
        className ?? "",
      ].join(" ")}
    >
      <span className="shrink-0 select-none text-muted-foreground" aria-hidden>
        https://{GIFTED_TEAM_WWW_PREFIX}
      </span>
      <span className="inline-flex min-w-0 items-baseline gap-0">
        <input
          id={id}
          type="text"
          inputMode="text"
          size={inputSize}
          className="min-w-0 border-0 bg-transparent px-0 py-2 text-foreground outline-none"
          placeholder="yourteam"
          value={value}
          onChange={(e) => onChange(sanitizeTeamSubdomainInput(e.target.value))}
          disabled={disabled}
          autoComplete="off"
          spellCheck={false}
        />
        <span
          className="shrink-0 select-none py-2 text-foreground"
          aria-hidden
        >
          .{GIFTED_SUBDOMAIN_APEX}
        </span>
      </span>
    </div>
  );
}
