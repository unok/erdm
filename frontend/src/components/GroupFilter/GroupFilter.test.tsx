import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GroupFilter } from './GroupFilter'

afterEach(() => {
  vi.clearAllMocks()
})

describe('GroupFilter', () => {
  it('renders nothing when there are no groups', () => {
    const { container } = render(
      <GroupFilter groups={[]} highlighted={new Set()} onChange={vi.fn()} />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('lists groups with their checked state reflecting the highlight set', () => {
    render(
      <GroupFilter groups={['auth', 'shop']} highlighted={new Set(['shop'])} onChange={vi.fn()} />,
    )
    const auth = screen.getByLabelText('auth') as HTMLInputElement
    const shop = screen.getByLabelText('shop') as HTMLInputElement
    expect(auth.checked).toBe(false)
    expect(shop.checked).toBe(true)
  })

  it('adds a group to the highlight set when toggled on', () => {
    const onChange = vi.fn()
    render(<GroupFilter groups={['auth', 'shop']} highlighted={new Set()} onChange={onChange} />)
    screen.getByLabelText('auth').click()
    expect(onChange).toHaveBeenCalledWith(new Set(['auth']))
  })

  it('removes a group from the highlight set when toggled off', () => {
    const onChange = vi.fn()
    render(
      <GroupFilter groups={['auth', 'shop']} highlighted={new Set(['auth'])} onChange={onChange} />,
    )
    screen.getByLabelText('auth').click()
    expect(onChange).toHaveBeenCalledWith(new Set())
  })

  it('clears all highlights via the Clear button', () => {
    const onChange = vi.fn()
    render(
      <GroupFilter
        groups={['auth', 'shop']}
        highlighted={new Set(['auth', 'shop'])}
        onChange={onChange}
      />,
    )
    screen.getByRole('button', { name: 'Clear' }).click()
    expect(onChange).toHaveBeenCalledWith(new Set())
  })
})
