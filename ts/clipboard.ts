export function copyToClipboard(elementId: string): void {
    const el = document.getElementById(elementId);
    if (el) {
        navigator.clipboard.writeText(el.textContent || '').catch(e =>
            console.error('Clipboard copy failed:', e)
        );
    }
}
