<script setup lang="ts">
import { useNav } from '@slidev/client'

const { currentPage, total } = useNav()
</script>

<template>
  <img
    v-if="currentPage !== 1 && currentPage !== total"
    src="/appicon.png"
    class="absolute top-5 right-6 h-9 rounded-lg opacity-80"
  />
</template>
