<script setup>
const props = defineProps({
  profilePicture: String,
  date: String,
  username: String,
  title: String,
  description: String,
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <img
        :src="profilePicture"
        alt="profile picture"
        class="profile-picture"
      />
      <div class="card-header-info">
        <h3>{{ username }}</h3>
        <p>{{ date }}</p>
      </div>
    </div>
    <div class="card-body">
      <h3>{{ title }}</h3>
      <p>{{ description }}</p>
    </div>
  </div>
</template>

<style scoped>
.card {
  display: flex;
  flex-direction: row;
  align-items: flex-start;
  border: 2px solid #00ff00;
  border-radius: 8px;
  padding: 1.5rem;
  width: 100%;
  max-width: 1000px;
  margin: 0 auto;
  box-shadow: 0 0 20px rgba(0, 255, 0, 0.3);
  background: linear-gradient(135deg, #1a1a1a 0%, #0d0d0d 100%);
  position: relative;
  overflow: hidden;
  transition: all 0.3s ease;
}

.card::before {
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
  transition: left 0.5s;
}

.card:hover::before {
  left: 100%;
}

.card:hover {
  transform: translateY(-2px);
  box-shadow: 0 0 30px rgba(0, 255, 0, 0.5);
  border-color: #00ff88;
}

.card-header {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 120px;
  min-width: 120px;
  gap: 0.5rem;
  margin-right: 1.5rem;
}

.profile-picture {
  width: 60px;
  height: 60px;
  border-radius: 50%;
  object-fit: cover;
  border: 2px solid #00ff00;
  box-shadow: 0 0 10px rgba(0, 255, 0, 0.5);
}

.card-header-info {
  text-align: center;
}

.card-header-info h3 {
  margin: 0;
  font-size: 0.9rem;
  font-weight: 600;
  color: #00ff00;
  text-shadow: 0 0 5px #00ff00;
  font-family: "Courier New", monospace;
}

.card-header-info p {
  margin: 0;
  font-size: 0.8rem;
  color: #888;
  font-family: "Courier New", monospace;
}

.card-body {
  flex: 1;
}

.card-body h3 {
  margin: 0 0 0.8rem 0;
  font-size: 1.3rem;
  font-weight: bold;
  color: #ff0000;
  text-shadow: 0 0 8px rgba(255, 0, 0, 0.5);
  font-family: "Courier New", monospace;
  letter-spacing: 1px;
}

.card-body p {
  margin: 0;
  color: #cccccc;
  line-height: 1.6;
  font-family: "Courier New", monospace;
  font-size: 0.95rem;
}

/* Animation de scan pour l'effet hacker */
@keyframes scan {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(100%);
  }
}

.card::after {
  content: "";
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 2px;
  background: linear-gradient(90deg, transparent, #00ff00, transparent);
  animation: scan 3s infinite;
  opacity: 0;
}

.card:hover::after {
  opacity: 1;
}
</style>
