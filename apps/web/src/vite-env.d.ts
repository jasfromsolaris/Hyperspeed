/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL?: string;
  /** Set to "true" to enable Yjs + y-monaco over the file collab WebSocket (see SpaceFilesPage). */
  readonly VITE_YJS_COLLAB?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
