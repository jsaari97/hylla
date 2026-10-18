<script lang="ts">
  import { onMount } from "svelte";
  import { GetCover } from "../bindings/github.com/jsaari97/hylla/libraryservice";

  export let id: string;
  export let title: string;
  export let format: string;
  let source = "";

  onMount(() => {
    if (format !== "epub" && format !== "pdf") return;
    const pending = GetCover(id);
    let mounted = true;
    void pending.then((cover) => {
      if (mounted) source = cover;
    }).catch(() => {});
    return () => {
      mounted = false;
      void pending.cancel().catch(() => {});
    };
  });
</script>

{#if source}
  <span class="image-cover">
    <img src={source} alt="Cover of {title}" onerror={() => source = ""} />
  </span>
{:else}
  <span class="format-cover" class:pdf={format === "pdf"} aria-hidden="true">
    <span class="cover-format">{format.toUpperCase()}</span>
    <span class="cover-lines"></span>
    <span class="cover-title">{title}</span>
  </span>
{/if}
