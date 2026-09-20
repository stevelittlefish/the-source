window.ui = SwaggerUIBundle({
 url: '/openapi.yaml',
 dom_id: '#swagger-ui',
 deepLinking: true,
 validatorUrl: null,
 queryConfigEnabled: false,
 supportedSubmitMethods: ['get', 'head'],
 displayRequestDuration: true,
 displayOperationId: true,
 defaultModelsExpandDepth: -1,
 presets: [SwaggerUIBundle.presets.apis],
 layout: 'BaseLayout',
});
