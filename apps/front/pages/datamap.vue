<template>
  <div class="h-screen w-screen flex flex-col bg-gray-900">
    <h1 class="text-3xl font-bold text-white m-0 p-0 leading-none">Data Map</h1>
    <p v-if="status" class="text-gray-300 p-2">{{ status }}</p>
    <div ref="networkContainer" class="flex-1 bg-gray-900 overflow-hidden network-container"></div>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { Network } from 'vis-network/standalone/esm/vis-network'

// Graphe CTI : entités (IOC, CVE, malware…) -> articles qui les mentionnent.
// Les bornes (articles, entités par article) sont appliquées par l'API.
const networkContainer = ref(null)
const status = ref('Chargement du graphe…')
const { public: { apiBase } } = useRuntimeConfig()

const colors = {
  Article: { background: '#94a3b8', border: '#64748b' },
  IP: { background: '#38bdf8', border: '#0ea5e9' },
  Domain: { background: '#34d399', border: '#10b981' },
  Hash: { background: '#a78bfa', border: '#8b5cf6' },
  CVE: { background: '#f87171', border: '#ef4444' },
  Malware: { background: '#fb923c', border: '#f97316' },
  ThreatActor: { background: '#facc15', border: '#eab308' },
  Infrastructure: { background: '#f472b6', border: '#ec4899' }
}

const options = {
  nodes: { shape: 'dot' },
  edges: {
    arrows: { to: { enabled: true, scaleFactor: 0.4 } },
    color: '#cccccc',
    smooth: true
  },
  physics: {
    enabled: true,
    solver: 'forceAtlas2Based',
    forceAtlas2Based: {
      gravitationalConstant: -50,
      centralGravity: 0.005,
      springLength: 230,
      springConstant: 0.18,
      damping: 0.4,
      avoidOverlap: 1
    },
    stabilization: {
      iterations: 200,
      fit: true
    }
  },
  layout: {
    improvedLayout: true
  },
  interaction: {
    dragView: true,
    zoomView: true,
    navigationButtons: true,
    keyboard: true
  }
}

onMounted(async () => {
  await nextTick()
  if (!networkContainer.value) {
    return
  }

  let graph
  try {
    const res = await fetch(`${apiBase}/graph`)
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`)
    }
    graph = await res.json()
  } catch (error) {
    console.error(error)
    status.value = 'Graphe indisponible : vérifier l\'API et Neo4j.'
    return
  }
  if (!graph.nodes?.length) {
    status.value = 'Aucune entité projetée pour le moment.'
    return
  }
  status.value = ''

  const nodes = graph.nodes.map(node => ({
    id: node.id,
    label: node.kind === 'Article' ? `article ${node.label.slice(-6)}` : node.label,
    title: `${node.kind} : ${node.label}`,
    shape: 'dot',
    size: node.kind === 'Article' ? 6 : 10,
    font: { size: 12, color: '#ffffff' },
    color: colors[node.kind] || colors.Article
  }))

  const networkInstance = new Network(networkContainer.value, { nodes, edges: graph.edges }, options)
  networkInstance.once('stabilizationIterationsDone', () => {
    networkInstance.setOptions({ physics: false })
    networkInstance.fit({
      animation: { duration: 500, easingFunction: 'easeInOutQuad' }
    })
  })
})
</script>

<style scoped>
.h-screen {
  height: 100vh;
}
.w-screen {
  width: 100vw;
}
.flex-1 {
  flex: 1 1 auto;
}
.bg-gray-900 {
  background-color: #111827;
}
.network-container {
  min-height: 0;
  height: 100%;
}
</style>
