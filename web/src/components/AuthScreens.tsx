import { KeyRound, Send } from "lucide-react";
import { BrandMark } from "./BrandMark";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Skeleton } from "./ui/skeleton";

function AuthShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="grid min-h-full place-items-center bg-background px-4 py-10">
      <div className="w-full max-w-[420px]">
        <Card className="border-border/80 nimbus-card-shadow-lg">
          <CardHeader className="space-y-4 pb-2">
            <BrandMark large />
            {children}
          </CardHeader>
        </Card>
        <p className="mt-4 text-center text-xs text-muted-foreground">
          Nimbus keeps your files cozy, private and always within reach.
        </p>
      </div>
    </div>
  );
}

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
      <AuthShell>
        <div className="grid gap-2 pt-2">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-4 w-1/2" />
        </div>
      </AuthShell>
    );
  }

  return (
    <AuthShell>
      <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-secondary text-secondary-foreground">
        <KeyRound className="h-5 w-5" />
      </div>
      <div>
        <CardTitle className="text-[22px]">Welcome back</CardTitle>
        <CardDescription className="mt-1.5">
          For private servers, paste the key from <code className="rounded bg-muted px-1 py-0.5 text-[12px]">data/access.secret</code>.
          On mobile you can open a link with <code className="rounded bg-muted px-1 py-0.5 text-[12px]">?access=…</code>.
        </CardDescription>
      </div>
      <CardContent className="grid gap-3 p-0 pt-2">
        <div className="grid gap-1.5">
          <Label htmlFor="access-key">Access key</Label>
          <Input
            id="access-key"
            type="password"
            value={props.value}
            onChange={(e) => props.onChange(e.target.value)}
            placeholder="Paste your access key"
            autoComplete="off"
            autoFocus
            onKeyDown={(e) => {
              if (e.key === "Enter" && props.value.trim()) props.onSubmit();
            }}
          />
        </div>
        <Button disabled={props.busy || !props.value.trim()} onClick={props.onSubmit}>
          {props.busy ? "Unlocking…" : "Unlock my drive"}
        </Button>
        {props.error && (
          <Alert variant="destructive">
            <AlertDescription>{props.error}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </AuthShell>
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
    <AuthShell>
      <div>
        <CardTitle className="text-[22px]">Connect Telegram</CardTitle>
        <CardDescription className="mt-1.5">
          One-time setup. Get credentials from{" "}
          <a
            className="font-medium text-primary underline-offset-4 hover:underline"
            href="https://my.telegram.org/apps"
            target="_blank"
            rel="noreferrer"
          >
            my.telegram.org/apps
          </a>{" "}
          — or set them in your server <code className="rounded bg-muted px-1 py-0.5 text-[12px]">.env</code> to skip this.
        </CardDescription>
      </div>
      <CardContent className="grid gap-3 p-0 pt-2">
        <div className="grid gap-1.5">
          <Label htmlFor="api-id">API ID</Label>
          <Input
            id="api-id"
            value={props.apiId}
            onChange={(e) => props.onApiId(e.target.value)}
            placeholder="12345678"
            inputMode="numeric"
            autoComplete="off"
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="api-hash">API Hash</Label>
          <Input
            id="api-hash"
            value={props.apiHash}
            onChange={(e) => props.onApiHash(e.target.value)}
            placeholder="Paste your api_hash"
            autoComplete="off"
          />
        </div>
        <Button disabled={props.busy || !props.apiId || !props.apiHash} onClick={props.onSave}>
          {props.busy ? "Connecting…" : "Continue"}
        </Button>
        {props.error && (
          <Alert variant="destructive">
            <AlertDescription>{props.error}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </AuthShell>
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
    <AuthShell>
      <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-[#229ED9] text-white shadow-sm">
        <Send className="h-5 w-5" />
      </div>
      <div>
        <CardTitle className="text-[22px]">Sign in with Telegram</CardTitle>
        <CardDescription className="mt-1.5">
          {codeStep
            ? "We sent you a code in Telegram — enter it below. Your session stays on this server."
            : "Use your Telegram phone number, just like in the Telegram app. No extra password needed."}
        </CardDescription>
      </div>
      <CardContent className="grid gap-3 p-0 pt-2">
        <div className="grid gap-1.5">
          <Label htmlFor="phone">Phone number</Label>
          <Input
            id="phone"
            value={props.phone}
            onChange={(e) => props.onPhone(e.target.value)}
            placeholder="+1 555 123 4567"
            inputMode="tel"
            autoComplete="tel"
            autoFocus={!codeStep}
            disabled={codeStep}
          />
        </div>
        {!codeStep ? (
          <Button
            className="bg-[#229ED9] hover:bg-[#1d8fc4]"
            disabled={props.busy || !props.phone.trim()}
            onClick={props.onSendCode}
          >
            <Send className="h-4 w-4" /> {props.busy ? "Sending…" : "Send login code"}
          </Button>
        ) : (
          <>
            <div className="grid gap-1.5">
              <Label htmlFor="code">Login code</Label>
              <Input
                id="code"
                value={props.code}
                onChange={(e) => props.onCode(e.target.value)}
                placeholder="12345"
                inputMode="numeric"
                autoComplete="one-time-code"
                autoFocus
              />
            </div>
            {props.need2fa && (
              <div className="grid gap-1.5">
                <Label htmlFor="pwd">2FA password</Label>
                <Input
                  id="pwd"
                  type="password"
                  value={props.password}
                  onChange={(e) => props.onPassword(e.target.value)}
                  autoComplete="current-password"
                />
              </div>
            )}
            <Button
              className="bg-[#229ED9] hover:bg-[#1d8fc4]"
              disabled={props.busy || !props.code.trim()}
              onClick={props.onSignIn}
            >
              {props.busy ? "Checking…" : "Continue"}
            </Button>
            <Button variant="ghost" disabled={props.busy} onClick={props.onChangeNumber}>
              Use a different number
            </Button>
          </>
        )}
        {props.error && (
          <Alert variant="destructive">
            <AlertDescription>{props.error}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </AuthShell>
  );
}

export function BootScreen() {
  return (
    <AuthShell>
      <div className="grid gap-2 pt-2">
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-4 w-1/2" />
      </div>
    </AuthShell>
  );
}
