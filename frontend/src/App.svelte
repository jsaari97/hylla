<script lang="ts">
  import {
    CreateLibrary,
    OpenLibrary,
  } from "../bindings/github.com/jsaari97/hylla/libraryservice";

  let libraryPath = "";
  let errorMessage = "";
  let busy = false;

  async function selectLibrary(action: () => Promise<string>) {
    busy = true;
    errorMessage = "";

    try {
      const selectedPath = await action();

      if (selectedPath !== "") {
        libraryPath = selectedPath;
      }
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <meta
    name="description"
    content="Hylla is a desktop application for managing a book library."
  />
</svelte:head>

<main class="setup-shell">
  <section class="setup-card" aria-labelledby="setup-title">
    {#if libraryPath}
      <h1 id="setup-title">Hylla</h1>

      <p class="description">
        Library opened at {libraryPath}
      </p>
    {:else}
      <h1 id="setup-title">Welcome to Hylla</h1>

      <p class="description">
        Create a library to keep your books, covers, and library data together
        in one place.
      </p>

      <div class="actions">
        <button
          type="button"
          disabled={busy}
          onclick={() => selectLibrary(CreateLibrary)}
        >
          Create Library
        </button>

        <button
          type="button"
          disabled={busy}
          onclick={() => selectLibrary(OpenLibrary)}
        >
          Open Library
        </button>
      </div>

      {#if errorMessage}
        <p class="error" role="alert">{errorMessage}</p>
      {/if}
    {/if}
  </section>
</main>
