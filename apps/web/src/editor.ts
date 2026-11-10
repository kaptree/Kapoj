import { basicSetup } from 'codemirror'
import { indentWithTab } from '@codemirror/commands'
import { indentUnit } from '@codemirror/language'
import { Compartment, EditorState, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { oneDark } from '@codemirror/theme-one-dark'
import { cpp } from '@codemirror/lang-cpp'
import { python } from '@codemirror/lang-python'
import { java } from '@codemirror/lang-java'
import { go } from '@codemirror/lang-go'

import { utf8Offset } from '@/utils/text'

export interface CodeEditorHandle {
  getValue(): string
  setValue(value: string): void
  getCursorByteOffset(): number
  insertAtCursor(text: string): void
  setLanguage(language: string): void
  focus(): void
  destroy(): void
}

export interface CodeEditorOptions {
  value: string
  language?: string
  readonly?: boolean
  onChange: (value: string) => void
}

/** Maps an editor language id to its syntax extension. */
function languageExtension(language: string | undefined): Extension {
  switch (language) {
    case 'cpp':
    case 'c':
      return cpp()
    case 'python':
      return python()
    case 'java':
      return java()
    case 'go':
      return go()
    default:
      return []
  }
}

export function createCodeEditor(host: HTMLElement, opts: CodeEditorOptions): CodeEditorHandle {
  const languageCompartment = new Compartment()

  const state = EditorState.create({
    doc: opts.value,
    extensions: [
      basicSetup,
      keymap.of([indentWithTab]),
      indentUnit.of('    '),
      EditorState.tabSize.of(4),
      EditorState.readOnly.of(!!opts.readonly),
      EditorView.editable.of(!opts.readonly),
      oneDark,
      EditorView.theme({
        '&': { height: '100%', fontSize: '14px' },
        '.cm-scroller': { fontFamily: 'var(--kapoj-font-mono)' },
        '&.cm-focused': { outline: 'none' },
      }),
      languageCompartment.of(languageExtension(opts.language)),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) opts.onChange(update.state.doc.toString())
      }),
    ],
  })

  const view = new EditorView({ state, parent: host })

  return {
    getValue: () => view.state.doc.toString(),
    setValue: (value) => {
      if (view.state.doc.toString() === value) return
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: value },
      })
    },
    getCursorByteOffset: () =>
      utf8Offset(view.state.doc.toString(), view.state.selection.main.head),
    insertAtCursor: (text) => {
      const head = view.state.selection.main.head
      view.dispatch({
        changes: { from: head, insert: text },
        selection: { anchor: head + text.length },
      })
      view.focus()
    },
    setLanguage: (language) => {
      view.dispatch({
        effects: languageCompartment.reconfigure(languageExtension(language)),
      })
    },
    focus: () => view.focus(),
    destroy: () => view.destroy(),
  }
}
