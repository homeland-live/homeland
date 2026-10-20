<script setup lang="ts">
import { ref, useTemplateRef, onMounted, onBeforeUnmount } from 'vue';
import { CodeJar } from 'codejar';
import { withLineNumbers } from 'codejar-linenumbers';
import Prism from 'prismjs';
import 'prismjs/themes/prism.css';
import 'codejar-linenumbers/es/codejar-linenumbers.css';

const props = defineProps<{ code?: string }>();

const editor = ref<CodeJar | undefined>();
const editorElem = useTemplateRef('editor');

onMounted(() => {
  if (!props.code) {
    return;
  }
  const options = {
    tab: ' '.repeat(2),
  };
  editor.value = CodeJar(
    editorElem.value as HTMLElement,
    withLineNumbers(Prism.highlightElement),
    options,
  );
  editor.value.updateCode(props.code);
  editorElem.value?.setAttribute('contenteditable', 'false');
});

onBeforeUnmount(() => {
  editor.value?.destroy();
});
</script>

<template>
  <div ref="editor" class="script-editor language-js" contenteditable="false" />
</template>

<style scoped>
.script-editor {
  background-color: #f7f7f7;
  border-radius: 6px;
  box-shadow:
    0 2px 2px 0 rgba(0, 0, 0, 0.14),
    0 1px 5px 0 rgba(0, 0, 0, 0.12),
    0 3px 1px -2px rgba(0, 0, 0, 0.2);
  font-family: monospace;
  font-size: 14px;
  font-weight: 400;
  height: 340px;
  letter-spacing: normal;
  line-height: 20px;
  padding: 10px;
  tab-size: 4;
}
</style>
