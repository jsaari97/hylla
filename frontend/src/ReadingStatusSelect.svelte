<script lang="ts">
  import { ReadingStatus } from "../bindings/github.com/jsaari97/hylla/internal/books/models";

  export let value: ReadingStatus;
  export let title: string;
  export let disabled = false;
  export let onChange: (status: ReadingStatus) => Promise<void>;
</script>

<select aria-label={`Reading status for ${title}`} {disabled} {value}
  onchange={(event) => {
    const select = event.currentTarget;
    const status = select.value as ReadingStatus;
    select.value = value;
    void onChange(status);
  }}>
  <option value={ReadingStatus.StatusUnread}>Unread</option>
  <option value={ReadingStatus.StatusReading}>Reading</option>
  <option value={ReadingStatus.StatusRead}>Read</option>
</select>

<style>
  select { width: 100%; min-height: 28px; padding: 3px; border: 1px solid #dce3dc; border-radius: 4px; background: #eef2ee; font-size: 11px; }
</style>
