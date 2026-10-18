<script lang="ts">
  import { onMount } from "svelte";
  import BookCover from "./BookCover.svelte";
  import ReadingStatusSelect from "./ReadingStatusSelect.svelte";
  import Icon from "./Icon.svelte";
  import { Events, type CancellablePromise } from "@wailsio/runtime";
  import { CreateLibrary, OpenLibrary, GetLibrary, ImportBooks, ImportFiles, SetReadingStatus } from "../bindings/github.com/jsaari97/hylla/libraryservice";
  import type { Book, ReadingStatus } from "../bindings/github.com/jsaari97/hylla/internal/books/models";
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
  let statusFilter = "all";
  let formatFilter = "all";
  let sortOrder = "recent";
  let bookView: "grid" | "list" = "grid";
  const statusViews = [
    { value: "all", label: "All books", icon: "books" as const },
    { value: "unread", label: "Unread", icon: "unread" as const },
    { value: "reading", label: "Reading", icon: "reading" as const },
    { value: "read", label: "Read", icon: "read" as const },
  ];
  $: viewTitle = statusViews.find((view) => view.value === statusFilter)?.label ?? "All books";
  $: libraryName = libraryPath.split(/[\\/]/).filter(Boolean).pop() ?? "Library";
  $: visibleBooks = books.filter((book) =>
    (statusFilter === "all" || book.ReadingStatus === statusFilter) &&
    (formatFilter === "all" || book.Format === formatFilter) &&
    `${book.Title} ${authorNames(book)} ${book.OriginalFilename}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
  ).sort((a, b) => {
    if (sortOrder === "title") return a.Title.localeCompare(b.Title) || a.ID.localeCompare(b.ID);
    if (sortOrder === "author") return authorNames(a).localeCompare(authorNames(b)) || a.Title.localeCompare(b.Title);
    return String(b.CreatedAt).localeCompare(String(a.CreatedAt)) || a.ID.localeCompare(b.ID);
  });
  $: selectedBook = visibleBooks.find((book) => book.ID === selectedID);
  $: importSummary = [
    { count: results.filter((result) => result.BookID && !result.Duplicate).length, label: "imported" },
    { count: results.filter((result) => result.Duplicate).length, label: "already in library" },
    { count: results.filter((result) => result.Error).length, label: "failed" },
  ].filter(({ count }) => count > 0).map(({ count, label }) => `${count} ${label}`).join(" · ");

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

  async function changeReadingStatus(book: Book, status: ReadingStatus) {
    if (busy || book.ReadingStatus === status) return;
    busy = true;
    errorMessage = "";
    try {
      const updated = await SetReadingStatus(book.ID, status);
      books = books.map((stored) => stored.ID === updated.ID ? updated : stored);
    } catch (error) {
      errorMessage = message(error);
    } finally {
      busy = false;
    }
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
    statusFilter = "all";
    formatFilter = "all";
    selectedID = id;
  }

  function clearFilters() {
    search = "";
    statusFilter = "all";
    formatFilter = "all";
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

<div class="app-frame">
  {#if libraryPath}
    <main class="library-shell" aria-busy={busy}>
      <aside class="sidebar" aria-label="Library navigation">
        <div class="window-drag-area" aria-hidden="true"></div>
        <nav aria-label="Reading status">
          {#each statusViews as view}
            <button type="button" class="nav-item" class:active={statusFilter === view.value}
              aria-pressed={statusFilter === view.value} onclick={() => statusFilter = view.value}>
              <Icon name={view.icon} /><span>{view.label}</span>
              <span class="count">{books.filter((book) => view.value === "all" || book.ReadingStatus === view.value).length}</span>
            </button>
          {/each}
        </nav>
        <p class="section-label formats-label">Formats</p>
        <nav aria-label="Book format">
          {#each [{ value: "all", label: "All formats" }, { value: "epub", label: "EPUB" }, { value: "pdf", label: "PDF" }] as format}
            <button type="button" class="nav-item format-item" class:active={formatFilter === format.value}
              aria-pressed={formatFilter === format.value} onclick={() => formatFilter = format.value}>
              <span>{format.label}</span>
            </button>
          {/each}
        </nav>
        <div class="library-location" title={libraryPath}>
          <Icon name="folder" size={18} />
          <strong>{libraryName}</strong>
        </div>
      </aside>

      <section class="workspace" aria-label="Book library">
        <header class="library-toolbar">
          <h1>{viewTitle}</h1>
          <label class="search-field">
            <Icon name="search" size={15} />
            <span class="sr-only">Search books</span>
            <input type="search" bind:value={search} placeholder="Search books" />
          </label>
          {#if cancelImport}
            <button type="button" class="secondary" disabled={cancelled} onclick={cancelImport}>
              {cancelled ? "Stopping…" : "Stop import"}
            </button>
          {/if}
          <button type="button" class="primary import-button" disabled={busy} onclick={() => importBooks(ImportBooks)}>
            <Icon name="plus" /><span>{cancelImport ? "Importing…" : "Import books"}</span>
          </button>
        </header>
        <div class="collection-toolbar">
          {#if search || formatFilter !== "all"}
            <span>{visibleBooks.length} {visibleBooks.length === 1 ? "book" : "books"}</span>
          {/if}
          <div class="collection-controls">
            <label class="sort-field">
              <span>Sort by</span>
              <select bind:value={sortOrder} aria-label="Sort books">
                <option value="recent">Date added</option>
                <option value="title">Title</option>
                <option value="author">Author</option>
              </select>
            </label>
            <div class="view-switch" role="group" aria-label="Book view">
              <button type="button" class:active={bookView === "grid"} aria-label="Grid view" title="Grid view"
                aria-pressed={bookView === "grid"} onclick={() => bookView = "grid"}><Icon name="grid" size={14} /></button>
              <button type="button" class:active={bookView === "list"} aria-label="List view" title="List view"
                aria-pressed={bookView === "list"} onclick={() => bookView = "list"}><Icon name="list" size={15} /></button>
            </div>
          </div>
        </div>
        <div class="collection-scroll">
          {#if errorMessage}<p class="error notice" role="alert">{errorMessage}</p>{/if}
          {#each warnings as warning}<p class="warning notice" role="status">{warning}</p>{/each}
          {#if cancelled}<p class="notice" role="status">Import stopped. Books already imported remain in your library.</p>{/if}
          {#if results.length}
            <section class="import-results notice" aria-label="Import results" aria-live="polite">
              <div class="notice-heading">
                <strong>{importSummary}</strong>
                <button type="button" class="icon-button" aria-label="Dismiss import results" onclick={() => results = []}><Icon name="close" size={14} /></button>
              </div>
              <details>
                <summary>Show details</summary>
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
              </details>
            </section>
          {/if}
          <section class="bookshelf" aria-label="Books">
            {#if books.length === 0}
              <div class="empty-state">
                <h2>No books yet</h2>
                <p>Import EPUB or PDF books, or drop files here.</p>
              </div>
            {:else if visibleBooks.length === 0}
              <div class="empty-state">
                <h2>No books found</h2>
                <button type="button" class="secondary" onclick={clearFilters}>Clear search and filters</button>
              </div>
            {:else}
              {#if bookView === "list"}
                <div class="list-heading" aria-hidden="true"><span></span><span>Title / Author</span><span>Format</span><span>Status</span></div>
              {/if}
              <div class:book-grid={bookView === "grid"} class:book-list={bookView === "list"}>
                {#each visibleBooks as book (book.ID)}
                  <div class="book-item" class:selected={selectedID === book.ID}>
                    <button type="button" class="book-card"
                      aria-pressed={selectedID === book.ID} onclick={() => selectedID = book.ID}>
                      <span class="cover-well"><BookCover id={book.ID} title={book.Title} format={book.Format} /></span>
                      <span class="book-summary">
                        <span class="book-title" title={book.Title}>{book.Title}</span>
                        <span class="book-author" title={authorNames(book)}>{authorNames(book) || "Unknown author"}</span>
                      </span>
                    </button>
                    <div class="book-caption">
                      <span class="format-label">{book.Format.toUpperCase()}</span>
                      <div class="book-status-control">
                        <ReadingStatusSelect value={book.ReadingStatus} title={book.Title} disabled={busy}
                          onChange={(status) => changeReadingStatus(book, status)} />
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </section>
        </div>
      </section>

      <aside class="inspector" aria-label="Book details">
          <div class="inspector-header"><h2>Book details</h2>
            {#if selectedBook}<button type="button" class="icon-button" aria-label="Clear book selection" onclick={() => selectedID = ""}><Icon name="close" size={15} /></button>{/if}
          </div>
        {#if selectedBook}
          <div class="book-details">
            <div class="detail-cover">{#key selectedBook.ID}<BookCover id={selectedBook.ID} title={selectedBook.Title} format={selectedBook.Format} />{/key}</div>
            <h3>{selectedBook.Title}</h3>
            <p class="detail-author">{authorNames(selectedBook) || "Unknown author"}</p>
            <div class="detail-status-control">
              <span>Reading status</span>
              <ReadingStatusSelect value={selectedBook.ReadingStatus} title={selectedBook.Title} disabled={busy}
                onChange={(status) => changeReadingStatus(selectedBook!, status)} />
            </div>
            <dl>
              <dt>Format</dt><dd>{selectedBook.Format.toUpperCase()}</dd>
              <dt>File name</dt><dd>{selectedBook.OriginalFilename}</dd>
              <dt>Date added</dt><dd>{new Date(selectedBook.CreatedAt).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })}</dd>
            </dl>
          </div>
        {:else}
          <div class="inspector-empty">
            <Icon name="reading" size={30} />
            <p>Select a book to see its details.</p>
          </div>
        {/if}
      </aside>
    </main>
  {:else}
    <main class="setup-shell" aria-busy={busy}>
      <div class="setup-drag-area window-drag-area" aria-hidden="true"></div>
      <section class="setup-card" aria-labelledby="setup-title">
        <span class="app-mark"><Icon name="books" size={34} /></span>
        <h1 id="setup-title">Welcome to Hylla</h1>
        <p class="description">Create a library or open an existing one.</p>
        <div class="setup-actions">
          <button type="button" class="primary" disabled={busy} onclick={() => selectLibrary(CreateLibrary)}><Icon name="plus" />Create library</button>
          <button type="button" class="secondary" disabled={busy} onclick={() => selectLibrary(OpenLibrary)}><Icon name="folder" />Open library</button>
        </div>
        {#if busy}<p class="setup-note" role="status">Opening your library…</p>{/if}
        {#if errorMessage}<p class="error notice" role="alert">{errorMessage}</p>{/if}
      </section>
    </main>
  {/if}
</div>
