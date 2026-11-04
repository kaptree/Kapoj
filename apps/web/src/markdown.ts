// Configures md-editor-v3 once for the whole app. By default the library pulls
// KaTeX, highlight.js and friends from a public CDN at runtime; here they are
// bundled instead, so pages work offline and never call out to third parties.
import { config } from 'md-editor-v3'
import katex from 'katex'
import hljs from 'highlight.js/lib/core'
import c from 'highlight.js/lib/languages/c'
import cpp from 'highlight.js/lib/languages/cpp'
import go from 'highlight.js/lib/languages/go'
import java from 'highlight.js/lib/languages/java'
import python from 'highlight.js/lib/languages/python'
import 'katex/dist/katex.min.css'
import 'md-editor-v3/lib/style.css'
// With a local highlight.js instance md-editor does not load any theme itself.
// Its code blocks are dark, so use the matching dark theme.
import 'highlight.js/styles/atom-one-dark.css'

hljs.registerLanguage('c', c)
hljs.registerLanguage('cpp', cpp)
hljs.registerLanguage('go', go)
hljs.registerLanguage('java', java)
hljs.registerLanguage('python', python)

config({
  editorExtensions: {
    katex: { instance: katex },
    highlight: { instance: hljs },
  },
})
