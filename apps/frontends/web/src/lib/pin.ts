// "Pin to top" through Document Picture-in-Picture: a small always-on-top
// window owned by this page. Chromium 116+; needs a user gesture.
import { mount, unmount, type Component } from 'svelte';

interface DocumentPictureInPicture {
  readonly window: Window | null;
  requestWindow(options?: { width?: number; height?: number }): Promise<Window>;
}
const pip = () => (window as Window & { documentPictureInPicture?: DocumentPictureInPicture }).documentPictureInPicture;

export const pinSupported = () => Boolean(pip());

/** Mount `component` in a new Picture-in-Picture window. `props` should be a
 * `$state` object so the pinned view follows later changes. Call from a click. */
export async function pinToTop<Props extends Record<string, unknown>>(
  component: Component<Props>, props: Props, title: string, onclose: () => void
): Promise<void> {
  const api = pip();
  if (!api) throw Error('Pin to top is not supported in this browser.');
  api.window?.close();
  const win = await api.requestWindow({ width: 360, height: 420 });
  // The new document starts unstyled: copy this page's styles once.
  for (const sheet of document.styleSheets) {
    try {
      const style = win.document.createElement('style');
      style.textContent = Array.from(sheet.cssRules, rule => rule.cssText).join('\n');
      win.document.head.append(style);
    } catch {
      if (!sheet.href) continue;
      const link = win.document.createElement('link');
      link.rel = 'stylesheet';
      link.href = sheet.href;
      win.document.head.append(link);
    }
  }
  win.document.title = title;
  const app = mount(component, { target: win.document.body, props });
  win.addEventListener('pagehide', () => { void unmount(app); onclose(); }, { once: true });
}
