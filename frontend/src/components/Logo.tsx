import { cn } from '@/lib/utils'

interface Props {
  className?: string
}

export default function Logo({ className }: Props) {
  return (
    <img
      src="/logo.svg"
      alt="Goink"
      className={cn('shrink-0', className)}
    />
  )
}
