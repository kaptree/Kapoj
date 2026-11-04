import type { components } from './schema'

type Schemas = components['schemas']

export type Me = Schemas['Me']
export type Profile = Schemas['Profile']
export type Role = Schemas['Role']
export type TokenPair = Schemas['TokenPair']
export type Session = Schemas['Session']
export type RegisterRequest = Schemas['RegisterRequest']
export type LoginRequest = Schemas['LoginRequest']
export type UpdateMeRequest = Schemas['UpdateMeRequest']
