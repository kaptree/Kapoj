import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

/**
 * kapOJ's PrimeVue preset.
 *
 * The palette follows the dark-first tech design system: cyan for brand and
 * interaction, slate surfaces, compact radii and a visible focus ring. The
 * matching CSS custom properties live in `src/styles/tokens.css`; keep both
 * files in sync when a design token changes.
 */
export const KapojPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '#e6fcff',
      100: '#c3f7ff',
      200: '#8cefff',
      300: '#4de4f7',
      400: '#00d9f5',
      500: '#00c4dd',
      600: '#009fb8',
      700: '#007f94',
      800: '#006474',
      900: '#004a57',
      950: '#002b34',
    },
    focusRing: {
      width: '2px',
      style: 'solid',
      color: '{primary.color}',
      offset: '2px',
      shadow: 'none',
    },
    colorScheme: {
      light: {
        surface: {
          0: '#ffffff',
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#475569',
          700: '#334155',
          800: '#1e293b',
          900: '#0f172a',
          950: '#020617',
        },
        primary: {
          color: '{primary.700}',
          contrastColor: '#ffffff',
          hoverColor: '{primary.800}',
          activeColor: '{primary.900}',
        },
        text: {
          color: '#0f172a',
          hoverColor: '#020617',
          mutedColor: '#64748b',
          hoverMutedColor: '#475569',
        },
      },
      dark: {
        surface: {
          0: '#ffffff',
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#334155',
          700: '#1e293b',
          800: '#111927',
          900: '#0c121c',
          950: '#070b12',
        },
        primary: {
          color: '{primary.400}',
          contrastColor: '#04121a',
          hoverColor: '{primary.300}',
          activeColor: '{primary.200}',
        },
        text: {
          color: '#e6edf3',
          hoverColor: '#f8fafc',
          mutedColor: '#94a3b8',
          hoverMutedColor: '#cbd5e1',
        },
      },
    },
  },
  components: {
    button: {
      root: {
        borderRadius: '{border.radius.sm}',
        transitionDuration: '160ms',
        label: {
          fontWeight: '600',
        },
      },
    },
    card: {
      root: {
        borderRadius: '{border.radius.md}',
        shadow: '0 1px 2px rgb(0 0 0 / 35%)',
      },
      body: {
        padding: '1.25rem',
      },
    },
  },
})
