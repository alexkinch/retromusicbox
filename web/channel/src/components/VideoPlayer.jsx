import React, { useRef, useEffect } from 'react'

export default function VideoPlayer({ video, onEnded, onError }) {
  const videoRef = useRef(null)
  const containerRef = useRef(null)
  const playPromiseRef = useRef(null)

  useEffect(() => {
    const el = videoRef.current
    if (!el || !video?.media_url) return

    // Fade out before loading
    if (containerRef.current) containerRef.current.style.opacity = '0'

    // If a previous play() is still pending, wait for it to settle before loading new source
    const load = () => {
      el.src = video.media_url
      el.muted = true

      // Wait for the video to be ready before playing
      const onCanPlay = () => {
        el.removeEventListener('canplay', onCanPlay)
        const promise = el.play()
        playPromiseRef.current = promise
        if (promise) {
          promise.then(() => {
            playPromiseRef.current = null
            if (containerRef.current) containerRef.current.style.opacity = '1'
            setTimeout(() => { el.muted = false }, 200)
          }).catch((err) => {
            playPromiseRef.current = null
            console.error('Autoplay failed:', err)
            if (containerRef.current) containerRef.current.style.opacity = '1'
          })
        }
      }
      el.addEventListener('canplay', onCanPlay)
      el.load()
    }

    // Wait for any pending play() to resolve before swapping source
    if (playPromiseRef.current) {
      playPromiseRef.current
        .then(load)
        .catch(load)
    } else {
      load()
    }

    return () => {
      // Cleanup: don't call pause directly, let the unmount handle it
    }
  }, [video?.media_url])

  return (
    <div
      ref={containerRef}
      className="video-player"
      style={{ opacity: 0, transition: 'opacity 0.5s ease' }}
    >
      <video
        ref={videoRef}
        className="video-element"
        playsInline
        onEnded={onEnded}
        onError={() => onError?.('Failed to load video')}
      />
    </div>
  )
}
