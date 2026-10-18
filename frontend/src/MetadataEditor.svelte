<script lang="ts">
  import type { Book } from "../bindings/github.com/jsaari97/hylla/internal/books/models";

  export let book: Book;
  export let busy = false;
  export let onSave: (title: string, authors: string[]) => Promise<void>;

  let editing = false;
  let title = "";
  let authors = "";
  let errorMessage = "";

  function startEditing() {
    title = book.Title;
    authors = (book.Authors ?? []).map((author) => author.Name).join("\n");
    errorMessage = "";
    editing = true;
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (busy) return;
    errorMessage = "";
    if (!title.trim()) {
      errorMessage = "Enter a title.";
      return;
    }
    try {
      await onSave(title, authors.split(/\r?\n/));
      editing = false;
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    }
  }
</script>

{#if editing}
  <form aria-label="Edit metadata" onsubmit={save}>
    <fieldset disabled={busy}>
      <label for="metadata-title">Title</label>
      <input id="metadata-title" type="text" bind:value={title} required />
      <label for="metadata-authors">Authors</label>
      <textarea id="metadata-authors" bind:value={authors} rows="4" aria-describedby="authors-help"></textarea>
      <p id="authors-help">Enter one author name per line.</p>
      {#if errorMessage}<p class="error" role="alert">{errorMessage}</p>{/if}
      <div class="actions">
        <button class="primary" type="submit">{busy ? "Saving…" : "Save"}</button>
        <button class="secondary" type="button" onclick={() => editing = false}>Cancel</button>
      </div>
    </fieldset>
  </form>
{:else}
  <button class="secondary edit-button" type="button" disabled={busy} onclick={startEditing}>Edit metadata</button>
{/if}

<style>
  form, .edit-button { margin-top: 16px; }
  fieldset { display: flex; flex-direction: column; gap: 8px; min-width: 0; margin: 0; padding: 0; border: 0; }
  label { font-size: 12px; font-weight: 600; }
  input, textarea { width: 100%; padding: 7px 8px; border: 1px solid #c8d3ca; border-radius: 4px; background: #fff; color: inherit; font: inherit; font-size: 12px; }
  textarea { resize: vertical; }
  p { font-size: 11px; line-height: 1.4; }
  .actions { display: flex; gap: 8px; margin-top: 4px; }
</style>
