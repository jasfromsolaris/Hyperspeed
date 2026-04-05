import { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";

import editorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";

// Minimal worker setup for Vite (text + JSON highlighting).
self.MonacoEnvironment = {
  getWorker(_workerId: string, _label: string) {
    return new editorWorker();
  },
};

loader.config({ monaco });
