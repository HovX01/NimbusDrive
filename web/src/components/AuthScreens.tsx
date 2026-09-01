import { BrandMark } from "./BrandMark";

type AccessProps = {
  value: string;
  busy: boolean;
  error: string;
  showAdvanced?: boolean;
  onChange: (v: string) => void;
  onSubmit: () => void;
};

export function AccessKeyScreen(props: AccessProps) {
  if (!props.showAdvanced) {
    return (
      <div className="auth-page">
        <div className="auth-panel quiet">
          <BrandMark large />
          <div className="skeleton line" />
          <div className="skeleton line short" />
        </div>
      </div>
    );
  }

  return (
    <div className="auth-page">
      <div className="auth-panel">
        <BrandMark large />
        <h1 className="auth-title">Server access key</h1>
        <p className="auth-copy">
          For private servers, paste the key from <code>data/access.secret</code>. On mobile you can open a link
          shared by the server admin with <code>?access=…</code> in the URL.
        </p>
        <div className="stack">
          <label>
            Access key
            <input
              type="password"
              value={props.value}
              onChange={(e) => props.onChange(e.target.value)}
              placeholder="hex from access.secret"
              autoComplete="off"
              autoFocus
              onKeyDown={(e) => {
                if (e.key === "Enter" && props.value.trim()) props.onSubmit();
              }}
            />
          </label>
          <button
            type="button"
            className="btn"
            disabled={props.busy || !props.value.trim()}
            onClick={props.onSubmit}
          >
            Unlock
          </button>
          {props.error && <div className="banner error">{props.error}</div>}
        </div>
      </div>
    </div>
  );
}

type SetupProps = {
  apiId: string;
  apiHash: string;
  busy: boolean;
  error: string;
  onApiId: (v: string) => void;
  onApiHash: (v: string) => void;
  onSave: () => void;
};

export function SetupScreen(props: SetupProps) {
  return (
    <div className="auth-page">
      <div className="auth-panel">
        <BrandMark large />
        <h1 className="auth-title">Connect Telegram API</h1>
        <p className="auth-copy">
          One-time server setup. Get credentials from{" "}
          <a href="https://my.telegram.org/apps" target="_blank" rel="noreferrer">
            my.telegram.org/apps
          </a>{" "}
          and set them in your server <code>.env</code> to skip this screen for everyone.
        </p>
        <div className="stack">
          <label>
            API ID
            <input
              value={props.apiId}
              onChange={(e) => props.onApiId(e.target.value)}
              placeholder="12345678"
              inputMode="numeric"
              autoComplete="off"
            />
          </label>
          <label>
            API Hash
            <input
              value={props.apiHash}
              onChange={(e) => props.onApiHash(e.target.value)}
              placeholder="hex string"
              autoComplete="off"
            />
          </label>
          <button
            type="button"
            className="btn-create"
            disabled={props.busy || !props.apiId || !props.apiHash}
            onClick={props.onSave}
          >
            Continue
          </button>
          {props.error && <div className="banner error">{props.error}</div>}
        </div>
      </div>
    </div>
  );
}

type LoginProps = {
  phone: string;
  code: string;
  password: string;
  phoneCodeHash: string;
  need2fa: boolean;
  busy: boolean;
  error: string;
  onPhone: (v: string) => void;
  onCode: (v: string) => void;
  onPassword: (v: string) => void;
  onSendCode: () => void;
  onSignIn: () => void;
  onChangeNumber: () => void;
};

export function LoginScreen(props: LoginProps) {
  const codeStep = Boolean(props.phoneCodeHash);

  return (
    <div className="auth-page">
      <div className="auth-panel auth-panel-telegram">
        <BrandMark large />
        <div className="auth-telegram-badge" aria-hidden>
          <i className="fa-brands fa-telegram" />
        </div>
        <h1 className="auth-title">Sign in with Telegram</h1>
        <p className="auth-copy">
          {codeStep
            ? "Enter the code Telegram sent you. Your session stays on this server."
            : "Use your Telegram phone number. Same login as the Telegram app — no extra passwords."}
        </p>
        <div className="stack">
          <label>
            Phone number
            <input
              value={props.phone}
              onChange={(e) => props.onPhone(e.target.value)}
              placeholder="+1 555 123 4567"
              inputMode="tel"
              autoComplete="tel"
              autoFocus={!codeStep}
              disabled={codeStep}
            />
          </label>
          {!codeStep ? (
            <button
              type="button"
              className="btn-create btn-telegram"
              disabled={props.busy || !props.phone.trim()}
              onClick={props.onSendCode}
            >
              <i className="fa-brands fa-telegram" /> Send login code
            </button>
          ) : (
            <>
              <label>
                Login code
                <input
                  value={props.code}
                  onChange={(e) => props.onCode(e.target.value)}
                  placeholder="12345"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  autoFocus
                />
              </label>
              {props.need2fa && (
                <label>
                  2FA password
                  <input
                    type="password"
                    value={props.password}
                    onChange={(e) => props.onPassword(e.target.value)}
                    autoComplete="current-password"
                  />
                </label>
              )}
              <button
                type="button"
                className="btn-create btn-telegram"
                disabled={props.busy || !props.code.trim()}
                onClick={props.onSignIn}
              >
                Continue
              </button>
              <button
                type="button"
                className="btn ghost auth-back"
                disabled={props.busy}
                onClick={props.onChangeNumber}
              >
                Use a different number
              </button>
            </>
          )}
          {props.error && <div className="banner error">{props.error}</div>}
        </div>
      </div>
    </div>
  );
}

export function BootScreen() {
  return (
    <div className="auth-page">
      <div className="auth-panel quiet">
        <BrandMark large />
        <div className="skeleton line" />
        <div className="skeleton line short" />
      </div>
    </div>
  );
}
