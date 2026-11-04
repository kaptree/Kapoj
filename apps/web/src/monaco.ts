import * as monaco from 'monaco-editor'
import EditorWorker from 'monaco-editor/editor/editor.worker.js?worker'

// Monaco loads workers from URLs by default. Use a bundled worker so the app
// works offline and never contacts a CDN.
;(window as unknown as { MonacoEnvironment: { getWorker: () => Worker } }).MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
}

export { monaco }
