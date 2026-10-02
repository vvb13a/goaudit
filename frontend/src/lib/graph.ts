// navigatorStorageKey is the local-storage key that remembers the last center
// node the user viewed for an audit, so returning to the navigator resumes
// rather than restarting.
export function navigatorStorageKey(auditId: string): string {
  return `graph-navigator:${auditId}`
}

// filetypeColor returns the canvas-friendly hex color of a filetype, mirroring
// the color families of filetypeClass for use in the force-graph renderer.
export function filetypeColor(filetype: string): string {
  switch (filetype) {
    case 'html':
      return '#2563eb'
    case 'css':
      return '#0ea5e9'
    case 'js':
      return '#d97706'
    case 'json':
    case 'xml':
      return '#7c3aed'
    case 'image':
    case 'jpg':
    case 'jpeg':
    case 'png':
    case 'webp':
    case 'gif':
    case 'svg':
    case 'ico':
    case 'avif':
    case 'bmp':
      return '#16a34a'
    case 'font':
      return '#c026d3'
    case 'video':
    case 'audio':
      return '#e11d48'
    case 'pdf':
      return '#dc2626'
    default:
      return '#64748b'
  }
}

// filetypeClass returns the Tailwind classes for the filetype pill of a graph
// node, grouping the common web types into readable color families.
export function filetypeClass(filetype: string): string {
  switch (filetype) {
    case 'html':
      return 'bg-blue-100 text-blue-700'
    case 'css':
      return 'bg-sky-100 text-sky-700'
    case 'js':
      return 'bg-amber-100 text-amber-700'
    case 'json':
    case 'xml':
      return 'bg-violet-100 text-violet-700'
    case 'image':
    case 'jpg':
    case 'jpeg':
    case 'png':
    case 'webp':
    case 'gif':
    case 'svg':
    case 'ico':
    case 'avif':
    case 'bmp':
      return 'bg-emerald-100 text-emerald-700'
    case 'font':
      return 'bg-fuchsia-100 text-fuchsia-700'
    case 'video':
    case 'audio':
      return 'bg-rose-100 text-rose-700'
    case 'pdf':
      return 'bg-red-100 text-red-700'
    default:
      return 'bg-slate-100 text-slate-600'
  }
}
