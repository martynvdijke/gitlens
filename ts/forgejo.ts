export function forgejoLogin(): void {
    const metaEl = document.querySelector<HTMLMetaElement>('meta[name="forgejo-default-url"]');
    // Alternatively, read from a data attribute or server-rendered variable
    // The default URL is currently rendered in the template as: {{.ForgejoDefaultURL}}
    // We use a scriptless approach: read from a hidden element injected by the server
    const defaultUrlEl = document.getElementById('forgejo-default-url');
    const defaultUrl = defaultUrlEl ? defaultUrlEl.getAttribute('data-url') : '';

    if (defaultUrl) {
        window.location.href = '/auth/forgejo?instance=' + encodeURIComponent(defaultUrl);
    } else {
        const url = prompt('Enter your Forgejo instance URL (e.g. https://codeberg.org):');
        if (url) {
            window.location.href = '/auth/forgejo?instance=' + encodeURIComponent(url);
        }
    }
}
