User-first design: design around user goals and workflows, not technical implementation.
Clear information hierarchy: users should immediately understand what is important, what they can do, and what happened.
Consistency: use a design system for colors, typography, spacing, buttons, inputs, cards, icons, and states.
Visual hierarchy: use size, weight, spacing, contrast, and positioning intentionally.
Responsive by default: design for mobile, tablet, and desktop rather than simply shrinking the desktop layout.
Accessibility: follow WCAG principles; keyboard navigation, focus states, semantic HTML, sufficient contrast, labels, ARIA only when necessary.
Interaction states: every interactive component should consider:
default
hover
focus
active
disabled
loading
success
error
empty
UX feedback: users should always understand whether an action succeeded, failed, or is still processing.
Forms: minimize friction, provide clear labels, validation, useful error messages, and preserve user input.
Navigation: users should always know where they are and how to get somewhere else.
Progressive disclosure: don't overwhelm users with information they don't need immediately.
Loading UX: use skeletons/placeholders where appropriate instead of unnecessary spinners.
Error UX: errors should explain the problem and what the user can do next.
Empty states: don't leave blank screens; explain what is missing and provide a useful action.
Design for real content: don't assume every username, title, image, or text has a perfect length.
Touch-friendly: interactive targets should be comfortable on mobile.
Performance: minimize unnecessary JavaScript, re-renders, network requests, large assets, and layout shifts.
Component architecture: create reusable components based on actual repeated patterns, not every <div>.
State management: keep state as local as possible; don't introduce global state unnecessarily.
Separation of concerns: UI components should not contain excessive business logic or API logic.
Design tokens: centralize colors, spacing, typography, radius, shadows, breakpoints, and other visual constants.
Semantic HTML: prefer native HTML elements before custom implementations.
Animation: animations should communicate state or hierarchy, not exist merely for decoration.
Respect reduced motion: support prefers-reduced-motion.
Typography: choose readable font sizes, line heights, weights, and widths.
Internationalization: avoid layouts that break with longer translations or different scripts.
Security: never trust frontend validation; protect sensitive operations on the backend.
Maintainability: avoid massive components, duplicated styles, arbitrary magic numbers, and deeply nested CSS.
Design-system rule

Do not invent a new visual style for every screen. Establish a small design system first, then compose screens from consistent primitives and components.

Frontend architecture rule

Keep components focused. Separate UI rendering, state management, API communication, business logic, and reusable utilities when complexity justifies it.

UX quality rule

Before considering a screen complete, evaluate the happy path, loading state, empty state, error state, disabled state, responsive behavior, accessibility, and edge cases.