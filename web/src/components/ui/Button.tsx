import { forwardRef } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { clsx } from 'clsx';
import type { ButtonHTMLAttributes } from 'react';

export const buttonVariants = cva('button', {
  variants: {
    variant: {
      primary: 'button--primary',
      secondary: 'button--secondary',
      ghost: 'button--ghost',
      danger: 'button--danger',
    },
    size: {
      sm: 'button--sm',
      md: 'button--md',
    },
  },
  defaultVariants: {
    variant: 'secondary',
    size: 'md',
  },
});

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof buttonVariants>;

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { className, variant, size, type = 'button', ...props },
  ref,
) {
  return <button ref={ref} type={type} className={clsx(buttonVariants({ variant, size }), className)} {...props} />;
});
