import { useCallback, useEffect, useRef } from "react";
import { createPortal } from "react-dom";

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  size?: "small" | "medium" | "large";
}

export function Modal({ isOpen, onClose, title, children, size = "medium" }: ModalProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);
  const hasInitialFocus = useRef(false);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  const stableOnClose = useCallback(() => onCloseRef.current(), []);

  useEffect(() => {
    if (!isOpen) {
      hasInitialFocus.current = false;
      return;
    }

    previouslyFocused.current = document.activeElement as HTMLElement | null;
    document.body.style.overflow = "hidden";

    function handleKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        stableOnClose();
      }
    }

    document.addEventListener("keydown", handleKey);

    // Focus first input in dialog only once on open
    if (!hasInitialFocus.current) {
      const dialog = dialogRef.current;
      if (dialog) {
        const firstInput = dialog.querySelector<HTMLElement>("input, select, textarea");
        if (firstInput) {
          firstInput.focus();
        }
      }
      hasInitialFocus.current = true;
    }

    return () => {
      document.removeEventListener("keydown", handleKey);
      document.body.style.overflow = "";
      if (previouslyFocused.current) {
        previouslyFocused.current.focus();
      }
    };
  }, [isOpen, stableOnClose]);

  if (!isOpen) return null;

  function handleOverlayClick(event: React.MouseEvent<HTMLDivElement>) {
    if (event.target === event.currentTarget) {
      onClose();
    }
  }

  return createPortal(
    <div className="modal-overlay" onClick={handleOverlayClick} role="presentation">
      <div
        ref={dialogRef}
        className={`modal-dialog modal-${size}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
      >
        <div className="modal-header">
          <h3 id="modal-title" className="modal-title">{title}</h3>
          <button className="modal-close" type="button" onClick={onClose} aria-label="Close">
            ×
          </button>
        </div>
        <div className="modal-body">{children}</div>
      </div>
    </div>,
    document.body,
  );
}
