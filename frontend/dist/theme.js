/* Sets the theme before the stylesheet paints. Doing this from app.js instead
   would show a flash of the light palette on every launch, since that runs at
   the end of the body. Falls back to the OS preference when the user has never
   chosen, and stays silent if storage is unavailable.

   It is a file of its own rather than inline in index.html so the page's
   Content-Security-Policy can refuse every inline script. */
(function () {
  try {
    var t = localStorage.getItem("sablewright.theme");
    if (t !== "light" && t !== "dark") {
      t = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    document.documentElement.setAttribute("data-theme", t);
  } catch (e) { /* no storage: the CSS media query still applies */ }
})();
