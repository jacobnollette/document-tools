pdf2md() {
  if [ $# -lt 1 ] || [ $# -gt 3 ]; then
    echo "Usage: pdf2md input.pdf [-o output.md]"
    return 1
  fi

  local in="$1"
  local out=""

  # Parse optional -o output
  if [ "$2" = "-o" ] && [ -n "$3" ]; then
    out="$3"
  else
    out="${in%.pdf}.md"
  fi

  pdftotext -layout "$in" - | sed 's/[ \t]*$//' > "$out"
}


md2pdf() {
  if [ $# -lt 1 ] || [ $# -gt 3 ]; then
    echo "Usage: md2pdf input.md [-o output.pdf]"
    return 1
  fi

  local in="$1"
  local out=""

  # Parse optional -o output
  if [ "$2" = "-o" ] && [ -n "$3" ]; then
    out="$3"
  else
    out="${in%.md}.pdf"
  fi

  pandoc "$in" -o "$out"
}
