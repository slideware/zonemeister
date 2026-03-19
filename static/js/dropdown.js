// Dropdown toggle and hamburger menu logic.
document.addEventListener("click", function (e) {
    // Dropdown toggle
    var toggle = e.target.closest(".dropdown-toggle");
    if (toggle) {
        e.preventDefault();
        var dropdown = toggle.closest(".dropdown");
        var isOpen = dropdown.hasAttribute("data-open");

        // Close all dropdowns first
        document.querySelectorAll(".dropdown[data-open]").forEach(function (d) {
            d.removeAttribute("data-open");
            d.querySelector(".dropdown-toggle").setAttribute("aria-expanded", "false");
        });

        if (!isOpen) {
            dropdown.setAttribute("data-open", "");
            toggle.setAttribute("aria-expanded", "true");
        }
        return;
    }

    // Hamburger toggle
    if (e.target.closest(".hamburger")) {
        var nav = document.querySelector(".nav-links");
        var btn = e.target.closest(".hamburger");
        if (nav.hasAttribute("data-open")) {
            nav.removeAttribute("data-open");
            btn.setAttribute("aria-expanded", "false");
        } else {
            nav.setAttribute("data-open", "");
            btn.setAttribute("aria-expanded", "true");
        }
        return;
    }

    // Click outside: close all dropdowns
    document.querySelectorAll(".dropdown[data-open]").forEach(function (d) {
        d.removeAttribute("data-open");
        d.querySelector(".dropdown-toggle").setAttribute("aria-expanded", "false");
    });
});

// Escape key: close all dropdowns and hamburger
document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") {
        document.querySelectorAll(".dropdown[data-open]").forEach(function (d) {
            d.removeAttribute("data-open");
            d.querySelector(".dropdown-toggle").setAttribute("aria-expanded", "false");
        });
        var nav = document.querySelector(".nav-links[data-open]");
        if (nav) {
            nav.removeAttribute("data-open");
            document.querySelector(".hamburger").setAttribute("aria-expanded", "false");
        }
    }
});
