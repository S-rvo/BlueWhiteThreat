<script setup>
import { ref } from "vue";

const isMenuOpen = ref(false);

const toggleMenu = () => {
  isMenuOpen.value = !isMenuOpen.value;
};
</script>

<template>
  <nav class="dark-navbar">
    <div class="nav-container">
      <!-- Logo avec effet glitch -->
      <div class="logo-section">
        <div class="logo-container">
          <h1 class="logo-text">🌑 DARK THREAT</h1>
          <div class="logo-subtitle">INTELLIGENCE NETWORK</div>
        </div>
        <div class="logo-glitch"></div>
      </div>

      <!-- Menu principal -->
      <div class="menu-section">
        <ul class="nav-menu">
          <li class="nav-item">
            <a href="#" class="nav-link active">
              <span class="link-icon">🏠</span>
              <span class="link-text">HOME</span>
            </a>
          </li>
          <li class="nav-item">
            <a href="#" class="nav-link">
              <span class="link-icon">💀</span>
              <span class="link-text">THREATS</span>
            </a>
          </li>
          <li class="nav-item">
            <a href="#" class="nav-link">
              <span class="link-icon">🔒</span>
              <span class="link-text">MARKET</span>
            </a>
          </li>
          <li class="nav-item">
            <a href="#" class="nav-link">
              <span class="link-icon">👥</span>
              <span class="link-text">FORUM</span>
            </a>
          </li>
          <li class="nav-item">
            <a href="#" class="nav-link">
              <span class="link-icon">⚡</span>
              <span class="link-text">TOOLS</span>
            </a>
          </li>
        </ul>
      </div>

      <!-- Statut de connexion -->
      <div class="status-section">
        <div class="connection-status">
          <div class="status-indicator online"></div>
          <span class="status-text">CONNECTED</span>
        </div>
        <div class="user-info">
          <span class="user-avatar">👤</span>
          <span class="username">ANONYMOUS_USER</span>
        </div>
      </div>

      <!-- Menu mobile -->
      <div class="mobile-menu-toggle" @click="toggleMenu">
        <div class="hamburger" :class="{ active: isMenuOpen }">
          <span></span>
          <span></span>
          <span></span>
        </div>
      </div>
    </div>

    <!-- Menu mobile déroulant -->
    <div class="mobile-menu" :class="{ open: isMenuOpen }">
      <ul class="mobile-nav-menu">
        <li><a href="#" class="mobile-nav-link">🏠 HOME</a></li>
        <li><a href="#" class="mobile-nav-link">💀 THREATS</a></li>
        <li><a href="#" class="mobile-nav-link">🔒 MARKET</a></li>
        <li><a href="#" class="mobile-nav-link">👥 FORUM</a></li>
        <li><a href="#" class="mobile-nav-link">⚡ TOOLS</a></li>
      </ul>
    </div>
  </nav>
</template>

<style scoped>
.dark-navbar {
  background: linear-gradient(90deg, #000000 0%, #1a1a1a 50%, #000000 100%);
  border-bottom: 2px solid #00ff00;
  box-shadow: 0 0 20px rgba(0, 255, 0, 0.3);
  position: relative;
  overflow: hidden;
  font-family: "Courier New", monospace;
}

.dark-navbar::before {
  content: "";
  position: absolute;
  top: 0;
  left: -100%;
  width: 100%;
  height: 100%;
  background: linear-gradient(
    90deg,
    transparent,
    rgba(0, 255, 0, 0.1),
    transparent
  );
  animation: scan-nav 4s infinite;
}

@keyframes scan-nav {
  0% {
    left: -100%;
  }
  50% {
    left: 100%;
  }
  100% {
    left: 100%;
  }
}

.nav-container {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 2rem;
  height: 80px;
  position: relative;
  z-index: 2;
}

/* Logo Section */
.logo-section {
  position: relative;
}

.logo-container {
  text-align: center;
}

.logo-text {
  margin: 0;
  font-size: 1.8rem;
  font-weight: bold;
  color: #00ff00;
  text-shadow: 0 0 10px #00ff00;
  letter-spacing: 2px;
  animation: pulse 2s infinite;
}

@keyframes pulse {
  0%,
  100% {
    text-shadow: 0 0 10px #00ff00;
  }
  50% {
    text-shadow: 0 0 20px #00ff00, 0 0 30px #00ff00;
  }
}

.logo-subtitle {
  font-size: 0.7rem;
  color: #888;
  margin-top: -5px;
  letter-spacing: 1px;
}

.logo-glitch {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: linear-gradient(90deg, transparent, #ff0000, transparent);
  opacity: 0;
  animation: glitch-nav 3s infinite;
}

@keyframes glitch-nav {
  0%,
  90%,
  100% {
    opacity: 0;
  }
  5%,
  85% {
    opacity: 0.3;
  }
}

/* Menu Section */
.menu-section {
  flex: 1;
  display: flex;
  justify-content: center;
}

.nav-menu {
  display: flex;
  list-style: none;
  margin: 0;
  padding: 0;
  gap: 2rem;
}

.nav-item {
  position: relative;
}

.nav-link {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-decoration: none;
  color: #888;
  padding: 0.5rem 1rem;
  border: 1px solid transparent;
  border-radius: 4px;
  transition: all 0.3s ease;
  position: relative;
  overflow: hidden;
}

.nav-link::before {
  content: "";
  position: absolute;
  top: 0;
  left: -100%;
  width: 100%;
  height: 100%;
  background: linear-gradient(
    90deg,
    transparent,
    rgba(0, 255, 0, 0.2),
    transparent
  );
  transition: left 0.3s;
}

.nav-link:hover::before {
  left: 100%;
}

.nav-link:hover,
.nav-link.active {
  color: #00ff00;
  border-color: #00ff00;
  box-shadow: 0 0 10px rgba(0, 255, 0, 0.5);
  transform: translateY(-2px);
}

.link-icon {
  font-size: 1.2rem;
  margin-bottom: 0.2rem;
}

.link-text {
  font-size: 0.8rem;
  font-weight: bold;
  letter-spacing: 1px;
}

/* Status Section */
.status-section {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.5rem;
}

.connection-status {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.status-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  animation: blink 1s infinite;
}

.status-indicator.online {
  background: #00ff00;
  box-shadow: 0 0 10px #00ff00;
}

@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0.3;
  }
}

.status-text {
  font-size: 0.7rem;
  color: #00ff00;
  font-weight: bold;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.user-avatar {
  font-size: 1rem;
}

.username {
  font-size: 0.8rem;
  color: #888;
  font-weight: bold;
}

/* Mobile Menu */
.mobile-menu-toggle {
  display: none;
  cursor: pointer;
}

.hamburger {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.hamburger span {
  width: 25px;
  height: 3px;
  background: #00ff00;
  transition: all 0.3s ease;
  box-shadow: 0 0 5px #00ff00;
}

.hamburger.active span:nth-child(1) {
  transform: rotate(45deg) translate(5px, 5px);
}

.hamburger.active span:nth-child(2) {
  opacity: 0;
}

.hamburger.active span:nth-child(3) {
  transform: rotate(-45deg) translate(7px, -6px);
}

.mobile-menu {
  display: none;
  background: rgba(0, 0, 0, 0.95);
  border-top: 1px solid #00ff00;
  max-height: 0;
  overflow: hidden;
  transition: max-height 0.3s ease;
}

.mobile-menu.open {
  max-height: 300px;
}

.mobile-nav-menu {
  list-style: none;
  margin: 0;
  padding: 1rem;
}

.mobile-nav-link {
  display: block;
  padding: 1rem;
  color: #888;
  text-decoration: none;
  border-bottom: 1px solid #333;
  transition: all 0.3s ease;
}

.mobile-nav-link:hover {
  color: #00ff00;
  background: rgba(0, 255, 0, 0.1);
}

/* Responsive */
@media (max-width: 768px) {
  .menu-section {
    display: none;
  }

  .status-section {
    display: none;
  }

  .mobile-menu-toggle {
    display: block;
  }

  .mobile-menu {
    display: block;
  }

  .logo-text {
    font-size: 1.4rem;
  }

  .nav-container {
    padding: 0 1rem;
  }
}
</style>
