/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./setagaya/ui/templates/**/*.html",
    "./setagaya/ui/static/js/**/*.js"
  ],
  theme: {
    extend: {
      colors: {
        'setagaya': {
          50: '#f0f9ff',
          100: '#e0f2fe',
          200: '#bae6fd',
          300: '#7dd3fc',
          400: '#38bdf8',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
          800: '#1e40af',
          900: '#1e3a8a'
        }
      }
    }
  },
  plugins: [
    require('@tailwindcss/forms'),
    require('@tailwindcss/typography')
  ]
}