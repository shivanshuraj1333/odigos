import { ROUTES } from '../constants';
import { handleExceptions, visitPage } from '../functions';

describe('Profiling page', () => {
  beforeEach(() => {
    cy.intercept('/graphql').as('gql');
    handleExceptions();
  });

  it('loads the continuous profiling route without client crash', () => {
    visitPage(ROUTES.PROFILING, () => {
      cy.get('body').should('not.be.empty');
      cy.contains('Continuous profiling', { timeout: 20000 }).should('be.visible');
    });
  });
});
