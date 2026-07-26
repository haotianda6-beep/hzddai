import { useEffect } from 'react'
import { useLocation } from 'react-router-dom'

const TARGET_SELECTOR = [
  ':scope > section',
  ':scope > div > section',
  ':scope article',
  ':scope form',
  ':scope [class*="card"]',
  ':scope [class*="Card"]',
  ':scope [class*="panel"]',
  ':scope [class*="Panel"]',
].join(',')

export function GlobalGsapMotion() {
  const location = useLocation()

  useEffect(() => {
    if (location.pathname === '/theme-preview' || location.pathname === '/auto-arbitrage') return
    const root = document.querySelector('main')
    if (!root) return

    let ctx: { revert: () => void } | undefined
    let cancelled = false
    const frame = window.requestAnimationFrame(() => {
      void import('gsap').then(({ default: gsap }) => {
        if (cancelled) return
        const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
        ctx = gsap.context(() => {
          const targets = gsap.utils.toArray<HTMLElement>(TARGET_SELECTOR).slice(0, 10)
          if (targets.length === 0) return
          gsap.set(targets, { willChange: 'transform, opacity' })
          gsap.fromTo(
            targets,
            { autoAlpha: reduceMotion ? 1 : 0, y: reduceMotion ? 0 : 8 },
            {
              autoAlpha: 1,
              y: 0,
              duration: reduceMotion ? 0 : 0.22,
              stagger: 0.012,
              ease: 'power2.out',
              overwrite: 'auto',
              clearProps: 'willChange',
            }
          )
        }, root)
      })
    })

    return () => {
      cancelled = true
      window.cancelAnimationFrame(frame)
      ctx?.revert()
    }
  }, [location.pathname])

  return null
}
