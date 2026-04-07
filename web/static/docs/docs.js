// docs.js — highlight the active sidebar link based on scroll position

document.addEventListener('DOMContentLoaded', () => {
  const sections = document.querySelectorAll('.docs-main [id]');
  const navLinks = document.querySelectorAll('.docs-nav a[href^="#"]');

  if (!sections.length || !navLinks.length) return;

  const observer = new IntersectionObserver(
    (entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return;
        navLinks.forEach((link) => {
          link.classList.toggle(
            'active',
            link.getAttribute('href') === '#' + entry.target.id
          );
        });
      });
    },
    { rootMargin: '0px 0px -70% 0px', threshold: 0 }
  );

  sections.forEach((section) => observer.observe(section));
});
