<script lang="ts">
  import { onMount } from "svelte";
  import BookCover from "./BookCover.svelte";
  import { Events, type CancellablePromise } from "@wailsio/runtime";
  import { CreateLibrary, OpenLibrary, GetLibrary, ImportBooks, ImportFiles } from "../bindings/github.com/jsaari97/hylla/libraryservice";
  import type { Book } from "../bindings/github.com/jsaari97/hylla/internal/books/models";
  import type { FileImportResult } from "../bindings/github.com/jsaari97/hylla/models";

  let libraryPath = "";
  let books: Book[] = [];
  let warnings: string[] = [];
  let results: FileImportResult[] = [];
  let errorMessage = "";
  let busy = true;
  let cancelled = false;
  let cancelImport: (() => void) | null = null;
  let search = "";
  let selectedID = "";
  $: visibleBooks = books.filter((book) =>
    `${book.Title} ${authorNames(book)} ${book.OriginalFilename}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
  );
  $: selectedBook = books.find((book) => book.ID === selectedID);

  function authorNames(book: Book) {
    return (book.Authors ?? []).map((author) => author.Name).join(", ");
  }

  function message(error: unknown) {
    return error instanceof Error ? error.message : String(error);
  }

  async function refreshLibrary() {
    const library = await GetLibrary();
    libraryPath = library.Path;
    books = library.Books ?? [];
    warnings = library.Warnings ?? [];
  }

  async function selectLibrary(action: () => Promise<string>) {
    busy = true;
    errorMessage = "";
    try {
      const path = await action();
      if (path) {
        libraryPath = path;
        await refreshLibrary();
      }
    } catch (error) {
      errorMessage = message(error);
    } finally {
      busy = false;
    }
  }

  async function importBooks(action: () => CancellablePromise<FileImportResult[] | null>) {
    if (busy) return;
    if (!libraryPath) {
      errorMessage = "Create or open a library before you import books.";
      return;
    }
    busy = true;
    cancelled = false;
    errorMessage = "";
    results = [];
    const pending = action();
    cancelImport = () => {
      cancelled = true;
      void pending.cancel().catch(() => {});
    };
    try {
      results = (await pending) ?? [];
      const imported = results.filter((result) => result.BookID);
      if (imported.length) showBook(imported[imported.length - 1].BookID);
    } catch (error) {
      if (!cancelled) errorMessage = message(error);
    } finally {
      cancelImport = null;
      try {
        await refreshLibrary();
      } catch (error) {
        errorMessage = message(error);
      }
      busy = false;
    }
  }

  function showBook(id: string) {
    search = "";
    selectedID = id;
  }

  onMount(() => {
    const unsubscribe = Events.On("books-dropped", (event) => {
      const paths: unknown = event.data;
      if (Array.isArray(paths) && paths.every((path) => typeof path === "string")) {
        void importBooks(() => ImportFiles(paths));
      }
    });
    void refreshLibrary()
      .catch((error) => { errorMessage = message(error); })
      .finally(() => { busy = false; });
    return unsubscribe;
  });
</script>

<svelte:head>
  <meta name="description" content="Hylla is a desktop application for managing a book library." />
</svelte:head>

{#if libraryPath}
  <main class="library-shell" aria-busy={busy}>
    <header class="library-header">
      <div><h1>Hylla</h1><p class="library-path">{libraryPath}</p></div>
      <div class="actions">
        {#if cancelImport}
          <button type="button" class="secondary" disabled={cancelled} onclick={cancelImport}>
            {cancelled ? "Stopping…" : "Stop import"}
          </button>
        {/if}
        <button type="button" disabled={busy} onclick={() => importBooks(ImportBooks)}>
          {cancelImport ? "Importing…" : "Import books"}
        </button>
      </div>
    </header>
    <div class="library-toolbar">
      <label class="search-field">
        <span>Search books</span>
        <input type="search" bind:value={search} placeholder="Title, author, or file name" />
      </label>
      <p>{books.length} {books.length === 1 ? "book" : "books"} in your library</p>
    </div>
    {#if errorMessage}<p class="error notice" role="alert">{errorMessage}</p>{/if}
    {#each warnings as warning}<p class="warning notice" role="status">{warning}</p>{/each}
    {#if cancelled}<p class="notice" role="status">Import stopped. Books already imported remain in your library.</p>{/if}
    {#if results.length}
      <section class="import-results notice" aria-label="Import results" aria-live="polite">
        <p>
          {results.filter((result) => result.BookID && !result.Duplicate).length} imported ·
          {results.filter((result) => result.Duplicate).length} already in library ·
          {results.filter((result) => result.Error).length} failed
        </p>
        <ul>
          {#each results as result}
            <li class:error={!!result.Error}>
              {#if result.Error}
                {result.Filename}: {result.Error}
              {:else}
                <button type="button" class="text-button" onclick={() => showBook(result.BookID)}>
                  {result.Title} — {result.Duplicate ? "Already in library" : "Imported"}
                </button>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}
    <div class="library-content">
      <section class="bookshelf" aria-label="Books">
        {#if books.length === 0}
          <div class="empty-library">
            <h2>Your library is ready</h2>
            <p>Import EPUB or PDF books, or drop files into this window.</p>
            <p>Hylla keeps a copy. Your source files stay unchanged.</p>
          </div>
        {:else if visibleBooks.length === 0}
          <p class="empty-library">No books match your search.</p>
        {:else}
          <div class="book-grid">
            {#each visibleBooks as book (book.ID)}
              <button type="button" class="book-card" class:selected={selectedID === book.ID}
                aria-pressed={selectedID === book.ID} onclick={() => selectedID = book.ID}>
                <BookCover id={book.ID} title={book.Title} format={book.Format} />
                <span class="book-title">{book.Title}</span>
                {#if authorNames(book)}<span class="book-author">{authorNames(book)}</span>{/if}
                <span class="book-caption">{book.Format.toUpperCase()} · {book.ReadingStatus}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>
      {#if selectedBook}
        <aside class="book-details" aria-label="Selected book">
          {#key selectedBook.ID}
            <BookCover id={selectedBook.ID} title={selectedBook.Title} format={selectedBook.Format} />
          {/key}
          <h2>{selectedBook.Title}</h2>
          <dl>
            {#if authorNames(selectedBook)}<dt>Authors</dt><dd>{authorNames(selectedBook)}</dd>{/if}
            <dt>File name</dt><dd>{selectedBook.OriginalFilename}</dd>
            <dt>Format</dt><dd>{selectedBook.Format.toUpperCase()}</dd>
            <dt>Reading status</dt><dd>{selectedBook.ReadingStatus}</dd>
          </dl>
          <p>Stored in your library.</p>
        </aside>
      {/if}
    </div>
  </main>
{:else}
  <main class="setup-shell">
    <section class="setup-card" aria-labelledby="setup-title">
      <h1 id="setup-title">Welcome to Hylla</h1>
      <p class="description">Create a library to keep your books, covers, and library data together in one place.</p>
      <div class="actions">
        <button type="button" disabled={busy} onclick={() => selectLibrary(CreateLibrary)}>Create Library</button>
        <button type="button" disabled={busy} onclick={() => selectLibrary(OpenLibrary)}>Open Library</button>
      </div>
      {#if errorMessage}<p class="error" role="alert">{errorMessage}</p>{/if}
    </section>
  </main>
{/if}
