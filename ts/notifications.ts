// Real-time activity notifications.
//
// The server pushes JSON messages over /ws/notifications. When browser
// notification permission is granted each message is surfaced as a native
// Notification; the feed view is refreshed in place so the item appears
// without a manual reload.

interface NotificationEvent {
    id: number;
    type: string;
    title: string;
    url: string;
    repo: string;
    timestamp: string;
}

interface NotificationMessage {
    type: string;
    event: NotificationEvent;
}

let socket: WebSocket | null = null;
let closedByUser = false;
let reconnectAttempt = 0;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

function websocketURL(): string {
    const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
    return `${scheme}://${location.host}/ws/notifications`;
}

function scheduleReconnect(): void {
    if (closedByUser) return;
    const base = Math.min(1000 * Math.pow(2, reconnectAttempt), 30000);
    const jitter = Math.floor(Math.random() * 500);
    reconnectAttempt += 1;
    if (reconnectTimer !== null) {
        clearTimeout(reconnectTimer);
    }
    reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connectNotifications();
    }, base + jitter);
}

function handleMessage(raw: string): void {
    let msg: NotificationMessage;
    try {
        msg = JSON.parse(raw) as NotificationMessage;
    } catch {
        return;
    }
    if (!msg || msg.type !== 'notification' || !msg.event) return;
    showNotification(msg.event);
    refreshFeed();
}

function showNotification(event: NotificationEvent): void {
    if (typeof Notification === 'undefined') return;
    if (Notification.permission !== 'granted') return;
    try {
        const n = new Notification(event.title, {
            body: `${event.type} · ${event.repo}`,
            icon: '/static/logo.svg',
            tag: `gitlens-event-${event.id}`,
        });
        n.onclick = () => {
            window.focus();
            if (event.url) {
                window.open(event.url, '_blank', 'noopener');
            }
            n.close();
        };
    } catch {
        // Some browsers throw when constructing a Notification outside a
        // service worker; ignore and rely on the feed refresh.
    }
}

function refreshFeed(): void {
    if (location.pathname !== '/feed') return;
    const target = document.getElementById('main-content');
    const htmx = (window as any).htmx;
    if (target && htmx && typeof htmx.ajax === 'function') {
        htmx.ajax('GET', '/feed', { target: '#main-content', swap: 'innerHTML' });
    }
}

export function connectNotifications(): void {
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
        return;
    }
    closedByUser = false;

    let ws: WebSocket;
    try {
        ws = new WebSocket(websocketURL());
    } catch {
        scheduleReconnect();
        return;
    }
    socket = ws;

    ws.onopen = () => {
        reconnectAttempt = 0;
    };
    ws.onmessage = (ev: MessageEvent) => {
        handleMessage(typeof ev.data === 'string' ? ev.data : '');
    };
    ws.onclose = () => {
        if (socket === ws) socket = null;
        scheduleReconnect();
    };
    ws.onerror = () => {
        try { ws.close(); } catch { /* ignore */ }
    };
}

export function disconnectNotifications(): void {
    closedByUser = true;
    if (reconnectTimer !== null) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
    }
    if (socket) {
        try { socket.close(); } catch { /* ignore */ }
        socket = null;
    }
}

// enableBrowserNotifications requests notification permission and returns a
// human-readable status string suitable for inline display.
export async function enableBrowserNotifications(): Promise<string> {
    if (typeof Notification === 'undefined') {
        return 'Browser notifications are not supported in this browser.';
    }
    if (Notification.permission === 'granted') {
        connectNotifications();
        return 'Browser notifications are enabled.';
    }
    if (Notification.permission === 'denied') {
        return 'Browser notifications are blocked. Allow them in your browser settings.';
    }
    try {
        const result = await Notification.requestPermission();
        if (result === 'granted') {
            connectNotifications();
            return 'Browser notifications are enabled.';
        }
        return 'Browser notifications were not enabled.';
    } catch {
        return 'Could not request notification permission.';
    }
}
