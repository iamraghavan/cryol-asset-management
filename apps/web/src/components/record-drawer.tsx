import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useRef } from 'react'
import type { ReactNode } from 'react'

export function RecordDrawer({ title, description, children, onClose }: {
  title: string; description: string; children: ReactNode; onClose: () => void
}) {
  const previousFocus = useRef(document.activeElement)
  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) onClose() }}>
      <Dialog.Portal>
        <Dialog.Overlay className="overlay" />
        <Dialog.Content className="drawer" onCloseAutoFocus={(event) => {
          if (previousFocus.current instanceof HTMLElement && previousFocus.current.isConnected) {
            event.preventDefault()
            previousFocus.current.focus()
          }
        }}>
          <header className="drawer-header">
            <div><Dialog.Title>{title}</Dialog.Title><Dialog.Description>{description}</Dialog.Description></div>
            <Dialog.Close asChild><button className="icon-button" aria-label="Close panel"><X size={19} /></button></Dialog.Close>
          </header>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
