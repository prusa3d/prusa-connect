import type { SessionStatus } from '../../data';
import type { ModulePrincipal } from '../../domain';

/** Who this browser is signed in as. */
export interface AccountSource {
  /** The subject the authorization graph is keyed by, such as `user:alice`. */
  readonly principal: ModulePrincipal | undefined;
  /** `unknown` until the first answer, which is what tells waiting from absent. */
  readonly status: SessionStatus;
}

/** The signed-in person's own account record. */
export interface AccountIdentity {
  /** Optional profile image supplied by the host application. */
  avatarUrl?: string | undefined;
  username?: string | undefined;
  displayName?: string | undefined;
  /** The org role, `viewer`, `operator` or `admin`. */
  role?: string | undefined;
  /** When the account was created, as the core wrote it. */
  createdAt?: string | undefined;
}

export interface SettingsAccountScreenProps {
  source: AccountSource;
  /** Optional host-owned save; absent when profile updates are unsupported. */
  onSaveIdentity?:
    | ((identity: {
        displayName: string;
        username: string;
      }) => Promise<void> | void)
    | undefined;
  /** Explains when a host saves only a local design preview. */
  identityPreview?: boolean;
  /** The account behind the subject. */
  identity?: AccountIdentity | undefined;
  /** True while the identity is being read. It waits rather than saying absent. */
  identityLoading?: boolean | undefined;
  /** True while a password change is in flight. It holds the submit. */
  changing?: boolean | undefined;
  /** What the change was refused with, as it was thrown. Drawn by `describeFault`. */
  fault?: unknown;
  /** True once a change was accepted. */
  changed?: boolean | undefined;
  /** True when the signed-in person has no password here, as at an identity provider. It hides the password form. */
  passwordless?: boolean | undefined;
  /** The reader asked for a new password. What that performs is the caller's. */
  onChangePassword: (current: string, next: string) => void;
}
