<script setup>
import Navbar from "./components/Navbar.vue";
import Card from "./components/Card.vue";
import { ref } from "vue";

const cards = ref([
  {
    profilePicture: "https://via.placeholder.com/150/ff0000/ffffff?text=H4CK3R",
    date: "2024-12-19",
    username: "ShadowBroker",
    title: "🔴 MEGA LEAK: Apple iCloud Database - 790K Records",
    description:
      "Fresh dump from Apple's infrastructure. Contains emails, phone numbers, addresses, and device IDs. Price: 50 BTC. Contact for samples. No time wasters.",
  },
  {
    profilePicture: "https://via.placeholder.com/150/00ff00/000000?text=CR4CK",
    date: "2024-12-18",
    username: "ZeroDayHunter",
    title: "💀 Facebook 533M Records - SQL Injection Exploit",
    description:
      "Successfully exploited Facebook's backend. Extracted 533M user records including phone numbers and personal data. Selling access to database. Serious buyers only.",
  },
  {
    profilePicture:
      "https://via.placeholder.com/150/0000ff/ffffff?text=PH4NT0M",
    date: "2024-12-17",
    username: "RansomLord",
    title: "💰 Colonial Pipeline - Full System Access",
    description:
      "Complete control over Colonial Pipeline systems. Ransomware deployed successfully. 4.4M USD paid in Bitcoin. Infrastructure still compromised. Looking for partners.",
  },
  {
    profilePicture: "https://via.placeholder.com/150/ffff00/000000?text=SPY",
    date: "2024-12-16",
    username: "APT_Operator",
    title: "🌐 SolarWinds Backdoor - Government Access",
    description:
      "Supply chain attack successful. Backdoor implanted in SolarWinds updates. Access to 18K+ organizations including government agencies. APT group confirmed. Selling access.",
  },
  {
    profilePicture: "https://via.placeholder.com/150/ff00ff/ffffff?text=LEAK",
    date: "2024-12-15",
    username: "DataVampire",
    title: "🩸 Ecole 2600 - Student Database Dump",
    description:
      "Complete student database from Ecole 2600. Personal info, grades, financial data. Massive security failure. Selling for 10 BTC. Includes admin access.",
  },
  {
    profilePicture: "https://via.placeholder.com/150/00ffff/000000?text=GH0ST",
    date: "2024-12-14",
    username: "CyberPhantom",
    title: "👻 Hospital Records - 2M Patient Data",
    description:
      "Major hospital chain compromised. Patient records, medical history, insurance info. HIPAA violation goldmine. Price negotiable. Discreet transaction.",
  },
  {
    profilePicture: "https://via.placeholder.com/150/ff8800/000000?text=W0LF",
    date: "2024-12-13",
    username: "WolfPack",
    title: "🐺 Banking Trojan - 50K Credit Cards",
    description:
      "Custom banking malware deployed. Successfully harvested 50K credit card details with CVV. High success rate. Looking for bulk buyers. Escrow accepted.",
  },
  {
    profilePicture: "https://via.placeholder.com/150/8800ff/ffffff?text=DEM0N",
    date: "2024-12-12",
    username: "DemonHacker",
    title: "😈 Government Email Breach - Diplomatic Cables",
    description:
      "Access to government email systems. Diplomatic communications, classified documents, intelligence reports. Highest bidder wins. No questions asked.",
  },
]);
</script>

<template>
  <div class="dark-web-container">
    <header>
      <Navbar />
    </header>
    <main>
      <div class="forum-header">
        <h1>🌑 DARK WEB FORUM - THREAT INTELLIGENCE</h1>
        <p class="forum-subtitle">Anonymous • Encrypted • Untraceable</p>
      </div>
      <div class="threads-container">
        <Card
          v-for="card in cards"
          :key="card.username + card.date"
          :profilePicture="card.profilePicture"
          :date="card.date"
          :username="card.username"
          :title="card.title"
          :description="card.description"
        />
      </div>
    </main>
  </div>
</template>

<style scoped>
.dark-web-container {
  min-height: 100vh;
  background: linear-gradient(135deg, #0a0a0a 0%, #1a1a1a 50%, #0d0d0d 100%);
  color: #00ff00;
  font-family: "Courier New", monospace;
}

header {
  width: 100%;
  height: 80px;
  border-bottom: 2px solid #00ff00;
  background: rgba(0, 0, 0, 0.9);
}

main {
  width: 100%;
  max-width: 1200px;
  margin: 0 auto;
  padding: 2rem;
}

.forum-header {
  text-align: center;
  margin-bottom: 3rem;
  padding: 2rem;
  background: rgba(0, 255, 0, 0.1);
  border: 1px solid #00ff00;
  border-radius: 8px;
}

.forum-header h1 {
  margin: 0;
  font-size: 2.5rem;
  font-weight: bold;
  text-shadow: 0 0 10px #00ff00;
  letter-spacing: 2px;
}

.forum-subtitle {
  margin: 1rem 0 0 0;
  font-size: 1.1rem;
  color: #888;
  font-style: italic;
}

.threads-container {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

/* Animation de glitch pour l'effet dark web */
@keyframes glitch {
  0% {
    text-shadow: 0 0 10px #00ff00;
  }
  25% {
    text-shadow: 2px 0 10px #ff0000;
  }
  50% {
    text-shadow: -2px 0 10px #0000ff;
  }
  75% {
    text-shadow: 2px 0 10px #ff0000;
  }
  100% {
    text-shadow: 0 0 10px #00ff00;
  }
}

.forum-header h1:hover {
  animation: glitch 0.3s infinite;
}
</style>
