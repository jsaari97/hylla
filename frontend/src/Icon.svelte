<script lang="ts">
  export let name: "books" | "search" | "plus" | "close" | "unread" | "reading" | "read" | "folder" | "grid" | "list";
  export let size = 16;
  const paths = {
    books: "M4 3h4v17H4zM8 5h4v15H8M15 3l4-1 4 17-4 1z",
    search: "M21 21l-5-5M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
    plus: "M12 5v14M5 12h14",
    close: "m6 6 12 12M6 18 18 6",
    unread: "M19 21l-7-4-7 4V3h14z",
    reading: "M12 5v16M3 3c4-1 6 0 9 2 3-2 5-3 9-2v16c-4-1-6 0-9 2-3-2-5-3-9-2z",
    read: "m7 12 3 3 7-7M22 12a10 10 0 1 1-10-10 10 10 0 0 1 10 10",
    folder: "M3 7V4h6l2 3h10v13H3z",
    grid: "M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z",
    list: "M3 4h4v4H3zM11 6h10M3 10h4v4H3zM11 12h10M3 16h4v4H3zM11 18h10",
  };
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
  stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d={paths[name]} />
</svg>
