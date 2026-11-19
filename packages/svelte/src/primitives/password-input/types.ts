import type { InputProps } from '../input/types';

export interface PasswordInputProps extends Omit<InputProps, 'type'> {
  /** The field name included in the visibility button label. */
  label?: string;
  /** Fill the column instead of stopping at the form-control width, as in a dialog whose other fields fill it. */
  fill?: boolean;
}
