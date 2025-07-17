<template>
  <div class="h-screen w-screen flex flex-col bg-gray-900">
    <h1 class="text-3xl font-bold text-white m-0 p-0 leading-none">Data Map</h1>
    <div ref="networkContainer" class="flex-1 bg-gray-900 overflow-hidden network-container"></div>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { Network } from 'vis-network/standalone/esm/vis-network'

const networkContainer = ref(null)

onMounted(async () => {
  await nextTick()

  if (!networkContainer.value) {
    console.error("Network container not found")
    return
  }

  const res = await fetch('http://localhost:8080/graph')
  const data = await res.json()

  console.log('Data API:', data)

  const nodesSet = new Set()
  const edges = []

  // Limiter à 200 sources
  const limitedData = data.slice(0, 200)

  limitedData.forEach(link => {
    nodesSet.add(link.source)

    // Limiter à 5 targets par source
    const limitedTargets = link.targets.slice(0, 5)

    limitedTargets.forEach(target => {
      nodesSet.add(target)

      // Ne pas créer de lien source ➔ source
      if (link.source !== target) {
        edges.push({ from: link.source, to: target })
      }
    })
  })

  const nodes = Array.from(nodesSet).map(url => ({
    id: url,
    label: url,
    shape: 'dot',
    size: 8,
    font: { size: 12, color: '#ffffff' },
    color: { background: '#38bdf8', border: '#0ea5e9' }
  }))

  const visData = { nodes, edges }

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


  setTimeout(() => {
    const networkInstance = new Network(networkContainer.value, visData, options)
    networkInstance.once('stabilizationIterationsDone', () => {
      networkInstance.setOptions({ physics: false })
      networkInstance.fit({
        animation: { duration: 500, easingFunction: 'easeInOutQuad' }
      })
    })
  }, 100)
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
