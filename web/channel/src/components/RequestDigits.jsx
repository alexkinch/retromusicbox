import React from 'react'

/**
 * Displays active phone request streams in the bottom-ticker slot.
 *
 * status values:
 *   dialling  -> phone icon + digits entered so far
 *   validated -> phone icon + digits (artist/title stay on the phone
 *                line via the IVR prompt; the original Box never
 *                flashed song details during the confirm step)
 *   success   -> phone icon + "Thanx!"
 *   fail      -> phone icon + "Try again"
 */
export default function RequestDigits({ callers }) {
  if (!callers || callers.length === 0) return null

  return (
    <div className="request-digits">
      {callers.map((caller) => (
        <div key={caller.id} className="ticker-line ticker-line-dial">
          <span className="request-phone-icon">&#9742;</span>
          {renderCallerBody(caller)}
        </div>
      ))}
    </div>
  )
}

function renderCallerBody(caller) {
  if (caller.status === 'success') {
    return <span className="request-result success">Thanx!</span>
  }
  if (caller.status === 'fail') {
    return <span className="request-result fail">Try again</span>
  }
  return <span className="request-digits-text">{caller.digits}</span>
}
