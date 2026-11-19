import type { SessionEnd } from '../../data';

/**
 * Why a sign-in at the identity provider did not finish, as the server's
 * callback names it in `sign_in_error`.
 */
export type SignInFailure =
  'provider' | 'id_token' | 'subject' | 'expired' | 'failed';

export interface SignInScreenProps {
  /** The server this browser is signed in to, drawn under the title. */
  host?: string | undefined;
  /** The configured instance title, when the public settings provide it. */
  instanceName?: string | undefined;
  /** Why the sign-in is showing, which decides the sentence above the form. */
  ended?: SessionEnd | undefined;
  /** True while a sign-in is in flight. */
  busy?: boolean | undefined;
  /** Why the last sign-in at the identity provider did not finish. */
  failure?: SignInFailure | undefined;
  /** The last refusal, as it was thrown. Rendered through `describeFault`. */
  error?: unknown;
  /**
   * The identity provider this server signs people in through. When it is
   * set the screen offers one button that calls `onRedirect` and no password
   * form.
   */
  redirectProvider?: string | undefined;
  onRedirect?: (() => void) | undefined;
  onSignIn: (username: string, password: string) => void;
}
